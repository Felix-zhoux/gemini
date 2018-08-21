// Copyright (C) 2018 ScyllaDB

package main

import (
	"fmt"
	"math/rand"

	"github.com/scylladb/gemini"
	"github.com/spf13/cobra"
)

var (
	testClusterHost   string
	oracleClusterHost string
	maxTests          int
	threads           int
	pkNumberPerThread int
	seed              int
	dropSchema        bool
	verbose           bool
)

type Status struct {
	WriteOps    int
	WriteErrors int
	ReadOps     int
	ReadErrors  int
}

func collectResults(one Status, sum Status) Status {
	sum.WriteOps += one.WriteOps
	sum.WriteErrors += one.WriteErrors
	sum.ReadOps += one.ReadOps
	sum.ReadErrors += one.ReadErrors
	return sum
}

func printResults(r Status) {
	fmt.Println("Results:")
	fmt.Printf("\twrite ops: %v\n", r.WriteOps)
	fmt.Printf("\twrite errors: %v\n", r.WriteErrors)
	fmt.Printf("\tread ops: %v\n", r.ReadOps)
	fmt.Printf("\tread errors: %v\n", r.ReadErrors)
}

func run(cmd *cobra.Command, args []string) {
	rand.Seed(int64(seed))
	fmt.Printf("Seed: %d\n", seed)
	fmt.Printf("Test cluster: %s\n", testClusterHost)
	fmt.Printf("Oracle cluster: %s\n", oracleClusterHost)

	session := gemini.NewSession(testClusterHost, oracleClusterHost)
	defer session.Close()

	schemaBuilder := gemini.NewSchemaBuilder()
	schemaBuilder.Keyspace(gemini.Keyspace{
		Name: "gemini",
	})
	schemaBuilder.Table(gemini.Table{
		Name: "data",
		PartitionKeys: []gemini.ColumnDef{
			{
				Name: "pk",
				Type: "int",
			},
		},
		ClusteringKeys: []gemini.ColumnDef{
			{
				Name: "ck",
				Type: "int",
			},
		},
		Columns: []gemini.ColumnDef{
			{
				Name: "n",
				Type: "blob",
			},
		},
	})
	schema := schemaBuilder.Build()
	if dropSchema {
		for _, stmt := range schema.GetDropSchema() {
			if verbose {
				fmt.Println(stmt)
			}
			if err := session.Mutate(stmt); err != nil {
				fmt.Printf("%v", err)
				return
			}
		}
	}
	for _, stmt := range schema.GetCreateSchema() {
		if verbose {
			fmt.Println(stmt)
		}
		if err := session.Mutate(stmt); err != nil {
			fmt.Printf("%v", err)
			return
		}
	}

	runJob(MixedJob, schema, session)
}

func runJob(f func(gemini.Schema, *gemini.Session, gemini.PartitionRange, chan Status), schema gemini.Schema, s *gemini.Session) {
	testRes := Status{}
	c := make(chan Status)
	minRange := 0
	maxRange := pkNumberPerThread

	for i := 0; i < threads; i++ {
		p := gemini.PartitionRange{Min: minRange + i*maxRange, Max: maxRange + i*maxRange}
		go f(schema, s, p, c)
	}

	for i := 0; i < threads; i++ {
		res := <-c
		testRes = collectResults(res, testRes)
	}

	printResults(testRes)
}

func MixedJob(schema gemini.Schema, s *gemini.Session, p gemini.PartitionRange, c chan Status) {
	testStatus := Status{}

	for i := 0; i < maxTests; i++ {
		mutateStmt := schema.GenMutateStmt(&p)
		mutateQuery := mutateStmt.Query
		mutateValues := mutateStmt.Values()
		if verbose {
			fmt.Printf("%s (values=%v)\n", mutateQuery, mutateValues)
		}
		testStatus.WriteOps++
		if err := s.Mutate(mutateQuery, mutateValues...); err != nil {
			fmt.Printf("Failed! Mutation '%s' (values=%v) caused an error: '%v'\n", mutateQuery, mutateValues, err)
			testStatus.WriteErrors++
		}

		checkStmt := schema.GenCheckStmt(&p)
		checkQuery := checkStmt.Query
		checkValues := checkStmt.Values()
		if verbose {
			fmt.Printf("%s (values=%v)\n", checkQuery, checkValues)
		}
		err := s.Check(checkQuery, checkValues...)
		if err == nil {
			testStatus.ReadOps++
		} else {
			if err != gemini.ErrReadNoDataReturned {
				fmt.Printf("Failed! Check '%s' (values=%v)\n%s\n", checkQuery, checkValues, err)
				testStatus.ReadErrors++
			}
		}
	}

	c <- testStatus
}

var rootCmd = &cobra.Command{
	Use:   "gemini",
	Short: "Gemini is an automatic random testing tool for Scylla.",
	Run:   run,
}

func Execute() {
}

func init() {
	rootCmd.Flags().StringVarP(&testClusterHost, "test-cluster", "t", "", "Host name of the test cluster that is system under test")
	rootCmd.MarkFlagRequired("test-cluster")
	rootCmd.Flags().StringVarP(&oracleClusterHost, "oracle-cluster", "o", "", "Host name of the oracle cluster that provides correct answers")
	rootCmd.MarkFlagRequired("oracle-cluster")
	rootCmd.Flags().IntVarP(&maxTests, "max-tests", "m", 100, "Maximum number of test iterations to run")
	rootCmd.Flags().IntVarP(&threads, "threads", "c", 10, "Number of threads to run concurrently")
	rootCmd.Flags().IntVarP(&pkNumberPerThread, "max-pk-per-thread", "p", 50, "Maximum number of partition keys per thread")
	rootCmd.Flags().IntVarP(&seed, "seed", "s", 1, "PRNG seed value")
	rootCmd.Flags().BoolVarP(&dropSchema, "drop-schema", "d", false, "Drop schema before starting tests run")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output during test run")
}

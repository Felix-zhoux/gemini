// Copyright 2019 ScyllaDB
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package typedef

import (
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/gocql/gocql"
	"golang.org/x/exp/rand"

	"github.com/scylladb/gemini/pkg/utils"
)

type BagType struct {
	ComplexType string     `json:"complex_type"` // We need to differentiate between sets and lists
	ValueType   SimpleType `json:"value_type"`
	Frozen      bool       `json:"frozen"`
}

func (ct *BagType) CQLType() gocql.TypeInfo {
	switch ct.ComplexType {
	case TYPE_SET:
		return goCQLTypeMap[gocql.TypeSet]
	default:
		return goCQLTypeMap[gocql.TypeList]
	}
}

func (ct *BagType) Name() string {
	if ct.Frozen {
		return "frozen<" + ct.ComplexType + "<" + ct.ValueType.Name() + ">>"
	}
	return ct.ComplexType + "<" + ct.ValueType.Name() + ">"
}

func (ct *BagType) CQLDef() string {
	if ct.Frozen {
		return "frozen<" + ct.ComplexType + "<" + ct.ValueType.Name() + ">>"
	}
	return ct.ComplexType + "<" + ct.ValueType.Name() + ">"
}

func (ct *BagType) CQLHolder() string {
	return "?"
}

func (ct *BagType) CQLPretty(query string, value []interface{}) (string, int) {
	if len(value) == 0 {
		return query, 0
	}
	if reflect.TypeOf(value[0]).Kind() != reflect.Slice {
		panic(fmt.Sprintf("set cql pretty, unknown type %v", ct))
	}
	s := reflect.ValueOf(value[0])
	op, cl := "[", "]"
	if ct.ComplexType == TYPE_SET {
		op, cl = "{", "}"
	}
	vv := op
	vv += strings.Repeat("?,", s.Len())
	vv = strings.TrimRight(vv, ",")
	vv += cl
	for i := 0; i < s.Len(); i++ {
		vv, _ = ct.ValueType.CQLPretty(vv, []interface{}{s.Index(i).Interface()})
	}
	return strings.Replace(query, "?", vv, 1), 1
}

func (ct *BagType) GenValue(r *rand.Rand, p *PartitionRangeConfig) []interface{} {
	count := utils.RandInt2(r, 1, maxBagSize+1)
	out := make([]interface{}, count)
	for i := 0; i < count; i++ {
		out[i] = ct.ValueType.GenValue(r, p)[0]
	}
	return []interface{}{out}
}

func (ct *BagType) GenJSONValue(r *rand.Rand, p *PartitionRangeConfig) interface{} {
	count := utils.RandInt2(r, 1, maxBagSize+1)
	out := make([]interface{}, count)
	for i := 0; i < count; i++ {
		out[i] = ct.ValueType.GenJSONValue(r, p)
	}
	return out
}

func (ct *BagType) LenValue() int {
	return 1
}

func (ct *BagType) Indexable() bool {
	return false
}

// ValueVariationsNumber returns number of bytes generated value holds
func (ct *BagType) ValueVariationsNumber(p *PartitionRangeConfig) float64 {
	return math.Pow(ct.ValueType.ValueVariationsNumber(p), maxBagSize)
}

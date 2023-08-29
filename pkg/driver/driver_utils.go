package driver

import (
	"github.com/eth-easl/loader/pkg/common"
)

func IsStringInList(str string, list []string) bool {
	for _, item := range list {
		if item == str {
			return true
		}
	}
	return false
}

func StartsWith(str, prefix string) bool {
	return len(str) >= len(prefix) && str[0:len(prefix)] == prefix
}

func FilterByKey(functions []*common.Function, key string) []*common.Function {
	filterFunctions := []*common.Function{}
	for _, function := range functions {
		// fmt.Printf("funct name %s, key %s\n", function.Name, key)
		if StartsWith(function.Name, key) {
			filterFunctions = append(filterFunctions, function)
		}
	}
	return filterFunctions
}

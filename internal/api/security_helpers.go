package api

import (
	"encoding/json"
	"fmt"
	"strconv"
)

func parseUintField(raw interface{}, field string) (uint, error) {
	switch value := raw.(type) {
	case float64:
		if value < 0 || value > float64(^uint32(0)) || value != float64(uint(value)) {
			return 0, fmt.Errorf("invalid %s", field)
		}
		return uint(value), nil
	case float32:
		v := float64(value)
		if v < 0 || v > float64(^uint32(0)) || v != float64(uint(v)) {
			return 0, fmt.Errorf("invalid %s", field)
		}
		return uint(v), nil
	case int:
		if value < 0 {
			return 0, fmt.Errorf("invalid %s", field)
		}
		return uint(value), nil
	case int32:
		if value < 0 {
			return 0, fmt.Errorf("invalid %s", field)
		}
		return uint(value), nil
	case int64:
		if value < 0 {
			return 0, fmt.Errorf("invalid %s", field)
		}
		return uint(value), nil
	case uint:
		return value, nil
	case uint32:
		return uint(value), nil
	case uint64:
		return uint(value), nil
	case json.Number:
		v, err := value.Int64()
		if err != nil || v < 0 {
			return 0, fmt.Errorf("invalid %s", field)
		}
		return uint(v), nil
	case string:
		v, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid %s", field)
		}
		return uint(v), nil
	default:
		return 0, fmt.Errorf("invalid %s", field)
	}
}

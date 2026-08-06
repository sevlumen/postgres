package postgres

import (
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"time"
)

type encodedArguments struct {
	values [][]byte
	nulls  []bool
}

func encodeArguments(args []driver.NamedValue) (*encodedArguments, error) {
	result := &encodedArguments{values: make([][]byte, len(args)), nulls: make([]bool, len(args))}
	for i, arg := range args {
		value, isNull, err := encodeText(arg.Value)
		if err != nil {
			return nil, fmt.Errorf("postgres: encode argument %d: %w", i+1, err)
		}
		result.values[i], result.nulls[i] = value, isNull
	}
	return result, nil
}

func normalizeValue(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	if driver.IsValue(value) {
		return value, nil
	}
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int8:
		return int64(typed), nil
	case int16:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case uint:
		return uint64ToInt64(uint64(typed))
	case uint8:
		return int64(typed), nil
	case uint16:
		return int64(typed), nil
	case uint32:
		return int64(typed), nil
	case uint64:
		return uint64ToInt64(typed)
	case fmt.Stringer:
		return typed.String(), nil
	}
	valueOf := reflect.ValueOf(value)
	if valueOf.Kind() == reflect.Array && valueOf.Len() == 16 && valueOf.Type().Elem().Kind() == reflect.Uint8 {
		bytes := make([]byte, 16)
		reflect.Copy(reflect.ValueOf(bytes), valueOf)
		return formatUUID(bytes), nil
	}
	return nil, fmt.Errorf("unsupported argument type %T", value)
}
func uint64ToInt64(value uint64) (int64, error) {
	if value > uint64(^uint64(0)>>1) {
		return 0, fmt.Errorf("uint64 %d overflows int64", value)
	}
	return int64(value), nil
}

func encodeText(value any) ([]byte, bool, error) {
	if value == nil {
		return nil, true, nil
	}
	switch typed := value.(type) {
	case string:
		return []byte(typed), false, nil
	case []byte:
		encoded := make([]byte, 2+hex.EncodedLen(len(typed)))
		copy(encoded, `\x`)
		hex.Encode(encoded[2:], typed)
		return encoded, false, nil
	case bool:
		return []byte(strconv.FormatBool(typed)), false, nil
	case int64:
		return []byte(strconv.FormatInt(typed, 10)), false, nil
	case float64:
		return []byte(strconv.FormatFloat(typed, 'g', -1, 64)), false, nil
	case time.Time:
		return []byte(typed.Format(time.RFC3339Nano)), false, nil
	case net.IP:
		return []byte(typed.String()), false, nil
	default:
		normalized, err := normalizeValue(value)
		if err != nil {
			return nil, false, err
		}
		if normalized == value {
			return nil, false, fmt.Errorf("unsupported argument type %T", value)
		}
		return encodeText(normalized)
	}
}

func formatUUID(value []byte) string {
	if len(value) != 16 {
		return ""
	}
	buffer := make([]byte, 36)
	hex.Encode(buffer[0:8], value[0:4])
	buffer[8] = '-'
	hex.Encode(buffer[9:13], value[4:6])
	buffer[13] = '-'
	hex.Encode(buffer[14:18], value[6:8])
	buffer[18] = '-'
	hex.Encode(buffer[19:23], value[8:10])
	buffer[23] = '-'
	hex.Encode(buffer[24:36], value[10:16])
	return string(buffer)
}

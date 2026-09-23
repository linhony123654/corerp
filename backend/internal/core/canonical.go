package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const MaxJSONSafeInteger int64 = 1<<53 - 1

// CanonicalJSON implements the RFC 8785 rules needed by CoreRP's closed M1
// hash documents: null, booleans, strings, signed integers, arrays, structs,
// and string-keyed maps. Floats are rejected so economic values cannot cross
// the contract's integer-only boundary through a hash helper.
func CanonicalJSON(value any) ([]byte, error) {
	var output bytes.Buffer
	if err := appendCanonical(&output, reflect.ValueOf(value)); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func HashJSON(value any) (string, error) {
	encoded, err := CanonicalJSON(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

type canonicalField struct {
	name  string
	value reflect.Value
}

func appendCanonical(output *bytes.Buffer, value reflect.Value) error {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer) {
		if value.IsNil() {
			output.WriteString("null")
			return nil
		}
		value = value.Elem()
	}
	if !value.IsValid() {
		output.WriteString("null")
		return nil
	}

	switch value.Kind() {
	case reflect.Bool:
		output.WriteString(strconv.FormatBool(value.Bool()))
	case reflect.String:
		return appendCanonicalString(output, value.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		number := value.Int()
		if number < -MaxJSONSafeInteger || number > MaxJSONSafeInteger {
			return NewError(CodeInvalidArgument, "canonical JSON integer exceeds the interoperable safe range")
		}
		output.WriteString(strconv.FormatInt(number, 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if value.Uint() > uint64(MaxJSONSafeInteger) {
			return NewError(CodeInvalidArgument, "canonical JSON integer exceeds the interoperable safe range")
		}
		output.WriteString(strconv.FormatUint(value.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		return NewError(CodeInvalidArgument, "canonical CoreRP hash documents cannot contain floats")
	case reflect.Slice, reflect.Array:
		output.WriteByte('[')
		for index := 0; index < value.Len(); index++ {
			if index > 0 {
				output.WriteByte(',')
			}
			if err := appendCanonical(output, value.Index(index)); err != nil {
				return err
			}
		}
		output.WriteByte(']')
	case reflect.Struct:
		fields := make([]canonicalField, 0, value.NumField())
		typeInfo := value.Type()
		for index := 0; index < value.NumField(); index++ {
			definition := typeInfo.Field(index)
			if definition.PkgPath != "" {
				continue
			}
			name, options, _ := strings.Cut(definition.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = definition.Name
			}
			fieldValue := value.Field(index)
			if strings.Contains(options, "omitempty") && fieldValue.IsZero() {
				continue
			}
			fields = append(fields, canonicalField{name: name, value: fieldValue})
		}
		return appendCanonicalObject(output, fields)
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return NewError(CodeInvalidArgument, "canonical JSON maps require string keys")
		}
		fields := make([]canonicalField, 0, value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			fields = append(fields, canonicalField{name: iterator.Key().String(), value: iterator.Value()})
		}
		return appendCanonicalObject(output, fields)
	default:
		return NewError(CodeInvalidArgument, fmt.Sprintf("unsupported canonical JSON kind %s", value.Kind()))
	}
	return nil
}

func appendCanonicalObject(output *bytes.Buffer, fields []canonicalField) error {
	sort.Slice(fields, func(left, right int) bool {
		return lessUTF16(fields[left].name, fields[right].name)
	})
	output.WriteByte('{')
	for index, field := range fields {
		if index > 0 {
			output.WriteByte(',')
		}
		if err := appendCanonicalString(output, field.name); err != nil {
			return err
		}
		output.WriteByte(':')
		if err := appendCanonical(output, field.value); err != nil {
			return err
		}
	}
	output.WriteByte('}')
	return nil
}

func lessUTF16(left, right string) bool {
	leftUnits := utf16.Encode([]rune(left))
	rightUnits := utf16.Encode([]rune(right))
	limit := len(leftUnits)
	if len(rightUnits) < limit {
		limit = len(rightUnits)
	}
	for index := 0; index < limit; index++ {
		if leftUnits[index] != rightUnits[index] {
			return leftUnits[index] < rightUnits[index]
		}
	}
	return len(leftUnits) < len(rightUnits)
}

func appendCanonicalString(output *bytes.Buffer, value string) error {
	if !utf8.ValidString(value) {
		return NewError(CodeInvalidArgument, "canonical JSON string contains invalid UTF-8")
	}
	output.WriteByte('"')
	for _, character := range value {
		switch character {
		case '"', '\\':
			output.WriteByte('\\')
			output.WriteRune(character)
		case '\b':
			output.WriteString(`\b`)
		case '\t':
			output.WriteString(`\t`)
		case '\n':
			output.WriteString(`\n`)
		case '\f':
			output.WriteString(`\f`)
		case '\r':
			output.WriteString(`\r`)
		default:
			if character < 0x20 {
				fmt.Fprintf(output, `\u%04x`, character)
			} else {
				output.WriteRune(character)
			}
		}
	}
	output.WriteByte('"')
	return nil
}

package webai

import (
	"encoding/binary"
	"errors"
)

type museProtoField struct {
	number int
	wire   uint64
	value  uint64
	bytes  []byte
}

func parseProto(data []byte) ([]museProtoField, error) {
	var fields []museProtoField
	for len(data) > 0 {
		key, n := binary.Uvarint(data)
		if n <= 0 || key>>3 == 0 {
			return nil, errors.New("invalid protobuf key")
		}
		data = data[n:]
		f := museProtoField{number: int(key >> 3), wire: key & 7}
		switch f.wire {
		case 0:
			v, n := binary.Uvarint(data)
			if n <= 0 {
				return nil, errors.New("invalid protobuf varint")
			}
			f.value, data = v, data[n:]
		case 1:
			if len(data) < 8 {
				return nil, errors.New("short protobuf fixed64")
			}
			data = data[8:]
		case 2:
			length, n := binary.Uvarint(data)
			if n <= 0 || length > uint64(len(data)-n) {
				return nil, errors.New("invalid protobuf length")
			}
			data = data[n:]
			f.bytes = data[:int(length)]
			data = data[int(length):]
		case 5:
			if len(data) < 4 {
				return nil, errors.New("short protobuf fixed32")
			}
			data = data[4:]
		default:
			return nil, errors.New("unsupported protobuf wire type")
		}
		fields = append(fields, f)
	}
	return fields, nil
}

func firstBytes(fields []museProtoField, number int) []byte {
	for _, f := range fields {
		if f.number == number && f.wire == 2 {
			return f.bytes
		}
	}
	return nil
}

func firstVarint(fields []museProtoField, number int) uint64 {
	for _, f := range fields {
		if f.number == number && f.wire == 0 {
			return f.value
		}
	}
	return 0
}

func protoUint(number int, value uint64) []byte {
	out := appendUvarint(nil, uint64(number<<3))
	return appendUvarint(out, value)
}

func protoBytes(number int, value []byte) []byte {
	out := appendUvarint(nil, uint64(number<<3|2))
	out = appendUvarint(out, uint64(len(value)))
	return append(out, value...)
}

func protoString(number int, value string) []byte { return protoBytes(number, []byte(value)) }

func appendUvarint(dst []byte, value uint64) []byte {
	for value >= 0x80 {
		dst = append(dst, byte(value)|0x80)
		value >>= 7
	}
	return append(dst, byte(value))
}

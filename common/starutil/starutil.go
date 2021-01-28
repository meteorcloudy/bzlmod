package starutil

import (
	"fmt"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkjson"
)

// ExtractStringSlice extracts a string slice out of the given starlark List. Throws an error if any list item is not a
// starlark String.
func ExtractStringSlice(list *starlark.List) ([]string, error) {
	if list == nil {
		return nil, nil
	}
	var r []string
	for i := 0; i < list.Len(); i++ {
		s, ok := starlark.AsString(list.Index(i))
		if !ok {
			return nil, fmt.Errorf("got %v, want string", list.Index(i).Type())
		}
		r = append(r, s)
	}
	return r, nil
}

// ValueHolder is a wrapper around a Starlark value that can be serialized and deserialized from/to JSON using Go's
// json package.
type ValueHolder struct {
	Value      starlark.Value
	Serialized []byte
}

// NewValueHolder creates a new ValueHolder around the given Starlark value. Returns an error if the Starlark value
// cannot be serialized.
func NewValueHolder(v starlark.Value) (*ValueHolder, error) {
	thread := starlark.Thread{Name: "marshaler"}
	s, err := starlark.Call(&thread, starlarkjson.Module.Members["encode"], []starlark.Value{v}, nil)
	if err != nil {
		return nil, err
	}
	return &ValueHolder{Value: v, Serialized: []byte(s.(starlark.String))}, nil
}

func (v *ValueHolder) MarshalJSON() ([]byte, error) {
	return v.Serialized, nil
}

func (v *ValueHolder) UnmarshalJSON(bytes []byte) error {
	thread := starlark.Thread{Name: "unmarshaler"}
	var err error
	v.Value, err = starlark.Call(&thread, starlarkjson.Module.Members["decode"], []starlark.Value{starlark.String(bytes)}, nil)
	v.Serialized = bytes
	return err
}

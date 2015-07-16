// Protobuf <-> Http

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	proto "github.com/golang/protobuf/proto"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

var ErrUnknownType = errors.New("unknown type")

var typmap map[string]reflect.Type

func init() {
	typmap = make(map[string]reflect.Type)
	typmap["VerifySessionResp"] = reflect.TypeOf(VerifySessionResp{})
	typmap["VerifySessionReq"] = reflect.TypeOf(VerifySessionReq{})
	typmap["PayNtf"] = reflect.TypeOf(PayNtf{})
	typmap["PayRsp"] = reflect.TypeOf(PayRsp{})
}

func findtyp(name string) reflect.Type {
	if typ, ok := typmap[name]; ok {
		return typ
	}
	return nil
}

func jsontopb(from []byte, to string) (proto.Message, error) {
	typ, ok := typmap[to]
	if !ok {
		return nil, ErrUnknownType
	}

	value := reflect.New(typ)
	if err := json.Unmarshal(from, value.Interface()); err != nil {
		return nil, err
	}

	return value.Interface().(proto.Message), nil
}

func pborigname(s string) string {
	// s := f.Tag.Get("protobuf")

	fields := strings.Split(s, ",")
	if len(fields) < 2 {
		logger.Fatal("tag has too few fields: %q\n", s)
	}

	for i := 2; i < len(fields); i++ {
		f := fields[i]
		switch {
		case strings.HasPrefix(f, "name="):
			return f[5:]
		}
	}

	logger.Fatal("invalid protofile !?")
	return ""
}

// pb is a pointer
func pbtohttpget(pb interface{}) ([]byte, error) {
	typ := reflect.TypeOf(pb).Elem()
	val := reflect.ValueOf(pb).Elem()
	buf := bytes.NewBuffer(nil)

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		// logger.Println(field.Name)
		if field.Name == "XXX_unrecognized" {
			continue
		}

		if val.Field(i).IsNil() {
			continue
		}

		buf.WriteString(pborigname(field.Tag.Get("protobuf")) + "=")

		switch field.Type.Kind() {
		case reflect.Ptr:
			elem_typ := field.Type.Elem()
			elem_val := val.Field(i).Elem()

			switch elem_typ.Kind() {
			case reflect.String:
				buf.WriteString(elem_val.String())
			}
		}

		buf.WriteString("&")
	}

	// logger.Println(string(buf.Bytes()))
	return buf.Bytes(), nil
}

// pb is a pointer
func httpgettopb(r *http.Request, pb interface{}) error {
	typ := reflect.TypeOf(pb).Elem()
	val := reflect.ValueOf(pb).Elem()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		// logger.Println(field.Name)
		if field.Name == "XXX_unrecognized" {
			continue
		}

		param_key := pborigname(field.Tag.Get("protobuf"))
		param_val := r.FormValue(param_key)
		if len(param_val) == 0 {
			continue
		}
		logger.Println("notify kv:", param_key, param_val)
		param_num, _ := strconv.Atoi(param_val)

		switch field.Type.Kind() {
		case reflect.Ptr:
			elem_typ := field.Type.Elem()
			elem_val := val.Field(i)

			switch elem_typ.Kind() {
			case reflect.String:
				elem_val.Set(reflect.ValueOf(proto.String(param_val)))
			case reflect.Int32, reflect.Int:
				elem_val.Set(reflect.ValueOf(proto.Int32(int32(param_num))))
			case reflect.Int64:
				elem_val.Set(reflect.ValueOf(proto.Int64(int64(param_num))))
			case reflect.Uint32:
				elem_val.Set(reflect.ValueOf(proto.Uint32(uint32(param_num))))
			case reflect.Uint64:
				elem_val.Set(reflect.ValueOf(proto.Uint64(uint64(param_num))))
			}
		}

	}
	return nil
}

func pbtojson(pb proto.Message) ([]byte, error) {
	return json.Marshal(pb)
}

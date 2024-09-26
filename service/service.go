package service

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

/***********服务注册***************/

// 一个具体的方法
type methodType struct {
	method    reflect.Method
	ArgType   reflect.Type
	ReplyType reflect.Type
	numCalls  uint64 // 统计调用次数
}

func (m *methodType) GetNumCalls() uint64 {
	return atomic.LoadUint64(&m.numCalls)
}

// 根据调用方法返回其输入输出参数类型
func (m *methodType) newArgv() reflect.Value {
	var argv reflect.Value
	if m.ArgType.Kind() == reflect.Ptr {
		argv = reflect.New(m.ArgType.Elem())
	} else {
		argv = reflect.New(m.ArgType).Elem()
	}
	return argv
}
func (m *methodType) newReplyv() reflect.Value {
	replyv := reflect.New(m.ReplyType.Elem())
	switch m.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	}
	return replyv
}

// 用于动态地将结构体的方法注册为RPC服务
type service struct {
	name     string        // 服务名称
	typ      reflect.Type  // reciver的type
	receiver reflect.Value // 结构体本身，保留是因为在调用时需要作为第 0 个参数
	method   map[string]*methodType
}

func newService(rcvr interface{}) *service {
	s := new(service)
	s.receiver = reflect.ValueOf(rcvr)
	// Indirect: If s.rcvr is not a pointer, Indirect returns s.rcvr.
	s.name = reflect.Indirect(s.receiver).Type().Name()
	s.typ = reflect.TypeOf(rcvr)
	if !ast.IsExported(s.name) {
		log.Fatalf("rpc server: %s is not a valid service name", s.name)
	}
	s.registerMethod()
	return s
}

// registerMethods 过滤出了符合条件的方法：
// - 两个导出或内置类型的入参（反射时为 3 个，第 0 个是自身，类似于 python 的 self，java 中的 this）
// - 返回值有且只有 1 个，类型为 error
func (s *service) registerMethod() {
	s.method = make(map[string]*methodType)
	for i := 0; i < s.typ.NumMethod(); i++ {
		method := s.typ.Method(i)
		mType := method.Type
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType, replyType := mType.In(1), mType.In(2)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		s.method[method.Name] = &methodType{
			method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}
		log.Printf("rpc server: register %s.%s\n", s.name, method.Name)
	}
}

func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

/***********通过反射值调用方法**************/
func (s *service) call(m *methodType, argv, replyv reflect.Value) error {
	atomic.AddUint64(&m.numCalls, 1)
	f := m.method.Func
	returnValues := f.Call([]reflect.Value{s.receiver, argv, replyv})
	if errInter := returnValues[0].Interface(); errInter != nil {
		return errInter.(error)
	}
	return nil
}

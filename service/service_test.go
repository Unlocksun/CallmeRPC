package service

import (
	"GeeRPC/foo"
	"fmt"
	"reflect"
	"testing"
)

func _assert(condition bool, msg string, v ...interface{}) {
	if !condition {
		panic(fmt.Sprintf("assertion failed: "+msg, v...))
	}
}

func TestNewService(t *testing.T) {
	var footest foo.Foo
	s := newService(&footest)
	_assert(len(s.method) == 1, "wrong service Method, expect 1, but got %d", len(s.method))
	mType := s.method["Sum"]
	_assert(mType != nil, "wrong Method, Sum shouldn't nil")
}

func TestMethodType_Call(t *testing.T) {
	var footest foo.Foo
	s := newService(&footest)
	mType := s.method["Sum"]

	argv := mType.newArgv()
	replyv := mType.newReplyv()
	argv.Set(reflect.ValueOf(foo.Args{Num1: 1, Num2: 3}))
	err := s.call(mType, argv, replyv)
	_assert(err == nil && *replyv.Interface().(*int) == 4 && mType.GetNumCalls() == 1, "failed to call Foo.Sum")
}

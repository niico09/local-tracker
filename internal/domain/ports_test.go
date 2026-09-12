package domain

import (
	"reflect"
	"testing"
)

// TestPortMethodsRequireActor walks every repository interface and fails when a
// method omits the Actor parameter, keeping I1 structural.
func TestPortMethodsRequireActor(t *testing.T) {
	actorType := reflect.TypeOf(Actor{})
	ports := []reflect.Type{
		reflect.TypeOf((*UserRepository)(nil)).Elem(),
		reflect.TypeOf((*SessionRepository)(nil)).Elem(),
	}
	for _, port := range ports {
		for i := 0; i < port.NumMethod(); i++ {
			method := port.Method(i)
			if !acceptsActor(method.Type, actorType) {
				t.Errorf("%s.%s does not declare an Actor parameter", port.Name(), method.Name)
			}
		}
	}
}

func acceptsActor(fn, actorType reflect.Type) bool {
	for i := 0; i < fn.NumIn(); i++ {
		if fn.In(i) == actorType {
			return true
		}
	}
	return false
}

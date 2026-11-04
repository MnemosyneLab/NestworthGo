package airuntime

import (
	"github.com/waltwang/nestworth-go/internal/assistant"
	"reflect"
	"testing"
)

func TestInstalledIPCDoesNotExposeCredentialsApprovalOrWrites(t *testing.T) {
	typ := reflect.TypeFor[*Service]()
	expected := map[string]bool{"Status": true, "List": true, "Create": true, "Inspect": true, "Messages": true, "Start": true, "Cancel": true, "Events": true, "Delete": true}
	if typ.NumMethod() != len(expected) {
		t.Fatal("review IPC inventory")
	}
	for i := 0; i < typ.NumMethod(); i++ {
		if !expected[typ.Method(i).Name] {
			t.Fatal("unsafe IPC method")
		}
	}
	s := NewService(nil)
	if s.Status().Available || s.Status().FinancialWrites {
		t.Fatal("closed service advertised authority")
	}
	if _, e := s.Start(assistant.StartRequest{}); e == nil {
		t.Fatal("unavailable runtime started")
	}
}

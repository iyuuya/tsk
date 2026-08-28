package task_test

import (
	"errors"
	"testing"

	"github.com/iyuuya/tsk/task"
)

func TestTaskDelegatesToAdaptor(t *testing.T) {
	a := &fakeAdaptor{kind: "mise", dir: "/proj"}
	tk := task.Task{Name: "build", Description: "compile", Adaptor: a}

	if got := tk.AdaptorName(); got != "mise" {
		t.Errorf("AdaptorName() = %q, want %q", got, "mise")
	}
	if got := tk.Dir(); got != "/proj" {
		t.Errorf("Dir() = %q, want %q", got, "/proj")
	}
	if err := tk.Run(); err != nil {
		t.Fatal(err)
	}
	if len(a.ran) != 1 || a.ran[0] != "build" {
		t.Errorf("Run() invoked adaptor with %v, want [build]", a.ran)
	}
}

func TestTaskRunPropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	tk := task.Task{Name: "x", Adaptor: &fakeAdaptor{err: wantErr}}
	if err := tk.Run(); !errors.Is(err, wantErr) {
		t.Errorf("Run() error = %v, want %v", err, wantErr)
	}
}

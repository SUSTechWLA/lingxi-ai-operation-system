package repository

import "testing"

func TestNewTaskFromMapCopiesAuthoritativeUserID(t *testing.T) {
	task := NewTaskFromMap(map[string]interface{}{"userId": "user-a", "query": "hello"})
	if task.UserID != "user-a" {
		t.Fatalf("UserID = %q, want user-a", task.UserID)
	}
}

func TestNewTaskFromMapAcceptsInternalSnakeCaseUserID(t *testing.T) {
	task := NewTaskFromMap(map[string]interface{}{"user_id": "user-b"})
	if task.UserID != "user-b" {
		t.Fatalf("UserID = %q, want user-b", task.UserID)
	}
}

package repository

import "testing"

func TestNewTaskForUserIgnoresUntrustedOwnerFields(t *testing.T) {
	task, err := NewTaskForUser("user-auth", map[string]interface{}{
		"userId":  "user-forged-camel",
		"user_id": "user-forged-snake",
		"query":   "hello",
	})
	if err != nil {
		t.Fatalf("NewTaskForUser: %v", err)
	}
	if task.UserID != "user-auth" {
		t.Fatalf("task user id = %q, want authenticated owner", task.UserID)
	}
	if task.Input["user_id"] != "user-auth" {
		t.Fatalf("sanitized input owner = %#v", task.Input["user_id"])
	}
	if _, exists := task.Input["userId"]; exists {
		t.Fatalf("untrusted camelCase owner remained in input: %#v", task.Input)
	}
}

func TestNewTaskForUserFailsClosedWithoutOwner(t *testing.T) {
	if _, err := NewTaskForUser("", map[string]interface{}{"userId": "forged"}); err == nil {
		t.Fatal("missing authenticated owner must fail closed")
	}
}

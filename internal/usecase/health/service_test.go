package health

import "testing"

func TestService_GetStatus(t *testing.T) {
	service := NewService()

	status := service.GetStatus()
	if status.Status != "ok" {
		t.Fatalf("expected status=ok, got %q", status.Status)
	}
}

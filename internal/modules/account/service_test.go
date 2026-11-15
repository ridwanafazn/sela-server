package account

import (
	"errors"
	"strings"
	"testing"

	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
)

func TestDeleteAccountValidation(t *testing.T) {
	service := &Service{}

	tests := []struct {
		name         string
		password     string
		confirmation string
		field        string
	}{
		{name: "wrong confirmation", password: "secret", confirmation: "delete", field: "confirmation"},
		{name: "missing password", password: "  ", confirmation: "DELETE", field: "password"},
		{name: "long password", password: strings.Repeat("a", maxPasswordLength+1), confirmation: "DELETE", field: "password"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.DeleteAccount(t.Context(), "user-id", test.password, test.confirmation)
			if err == nil {
				t.Fatal("expected an error")
			}
			var apiErr *apierrors.Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want *apierrors.Error", err)
			}
			if apiErr.Status != 400 {
				t.Errorf("status = %d, want 400", apiErr.Status)
			}
			if len(apiErr.Details) == 0 || apiErr.Details[0].Field != test.field {
				t.Errorf("details = %+v, want field %s", apiErr.Details, test.field)
			}
		})
	}
}

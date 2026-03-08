package credential

import (
	"context"
	"errors"
	"testing"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
)

type statusTestRepo struct {
	profile   *domaincredential.Profile
	getErr    error
	updateErr error
	updated   []domaincredential.Profile
}

func (r *statusTestRepo) Save(_ context.Context, p domaincredential.Profile) (domaincredential.Profile, error) {
	return p, nil
}

func (r *statusTestRepo) List(_ context.Context) ([]domaincredential.Profile, error) {
	return nil, nil
}

func (r *statusTestRepo) GetByID(_ context.Context, _ string) (*domaincredential.Profile, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.profile == nil {
		return nil, nil
	}
	p := *r.profile
	return &p, nil
}

func (r *statusTestRepo) Update(_ context.Context, p domaincredential.Profile) (domaincredential.Profile, error) {
	if r.updateErr != nil {
		return domaincredential.Profile{}, r.updateErr
	}
	r.updated = append(r.updated, p)
	return p, nil
}

func (r *statusTestRepo) UpsertByTypeAccount(_ context.Context, p domaincredential.Profile) (domaincredential.Profile, error) {
	return p, nil
}

func (r *statusTestRepo) ListEnabled(_ context.Context) ([]domaincredential.Profile, error) {
	return nil, nil
}

func (r *statusTestRepo) CountEnabled(_ context.Context) (int64, error) {
	return 0, nil
}

func (r *statusTestRepo) RandomSample(_ context.Context, sampleSize int, seed int64) ([]domaincredential.Profile, error) {
	return nil, nil
}

func TestStatusService_SetEnabled(t *testing.T) {
	t.Run("updates enabled to false", func(t *testing.T) {
		repo := &statusTestRepo{profile: &domaincredential.Profile{ID: "cred-1", Enabled: true}}
		svc := NewStatusService(repo)

		updated, err := svc.SetEnabled(context.Background(), "cred-1", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if updated.Enabled {
			t.Fatal("expected enabled=false")
		}
		if len(repo.updated) != 1 {
			t.Fatalf("expected one update call, got %d", len(repo.updated))
		}
		if repo.updated[0].Enabled {
			t.Fatal("expected persisted profile enabled=false")
		}
	})

	t.Run("updates enabled to true", func(t *testing.T) {
		repo := &statusTestRepo{profile: &domaincredential.Profile{ID: "cred-1", Enabled: false}}
		svc := NewStatusService(repo)

		updated, err := svc.SetEnabled(context.Background(), "cred-1", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !updated.Enabled {
			t.Fatal("expected enabled=true")
		}
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := &statusTestRepo{}
		svc := NewStatusService(repo)

		_, err := svc.SetEnabled(context.Background(), "missing", false)
		if !errors.Is(err, ErrCredentialNotFound) {
			t.Fatalf("expected ErrCredentialNotFound, got %v", err)
		}
	})

	t.Run("returns error on empty id", func(t *testing.T) {
		repo := &statusTestRepo{}
		svc := NewStatusService(repo)

		_, err := svc.SetEnabled(context.Background(), "   ", false)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns wrapped repo get error", func(t *testing.T) {
		repo := &statusTestRepo{getErr: errors.New("db down")}
		svc := NewStatusService(repo)

		_, err := svc.SetEnabled(context.Background(), "cred-1", false)
		if err == nil || err.Error() == "db down" {
			t.Fatalf("expected wrapped error, got %v", err)
		}
	})

	t.Run("returns wrapped repo update error", func(t *testing.T) {
		repo := &statusTestRepo{
			profile:   &domaincredential.Profile{ID: "cred-1", Enabled: true},
			updateErr: errors.New("write failed"),
		}
		svc := NewStatusService(repo)

		_, err := svc.SetEnabled(context.Background(), "cred-1", false)
		if err == nil || err.Error() == "write failed" {
			t.Fatalf("expected wrapped error, got %v", err)
		}
	})
}

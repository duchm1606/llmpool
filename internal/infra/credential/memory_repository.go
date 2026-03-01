package credential

import (
	"context"
	"sync"

	domaincredential "github.com/duchoang/llmpool/internal/domain/credential"
)

type MemoryRepository struct {
	mu       sync.RWMutex
	profiles map[string]domaincredential.Profile
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{profiles: make(map[string]domaincredential.Profile)}
}

func (r *MemoryRepository) Save(_ context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.profiles[profile.ID] = profile
	return profile, nil
}

func (r *MemoryRepository) List(_ context.Context) ([]domaincredential.Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domaincredential.Profile, 0, len(r.profiles))
	for _, p := range r.profiles {
		out = append(out, p)
	}

	return out, nil
}

func (r *MemoryRepository) Update(_ context.Context, profile domaincredential.Profile) (domaincredential.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.profiles[profile.ID] = profile
	return profile, nil
}

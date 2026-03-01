package health

import domainhealth "github.com/duchoang/llmpool/internal/domain/health"

type Service interface {
	GetStatus() domainhealth.Status
}

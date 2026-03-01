package health

import domainhealth "github.com/duchoang/llmpool/internal/domain/health"

type service struct{}

func NewService() Service {
	return &service{}
}

func (s *service) GetStatus() domainhealth.Status {
	return domainhealth.Status{Status: "ok"}
}

package questionbank

import "interview-agent/internal/llm"

type GenerationService struct {
	imports ImportStore
	writer  Writer
	model   llm.ChatModel
}

type GenerationServiceDeps struct {
	Imports ImportStore
	Writer  Writer
	Model   llm.ChatModel
}

func NewGenerationService(deps GenerationServiceDeps) *GenerationService {
	return &GenerationService{
		imports: deps.Imports,
		writer:  deps.Writer,
		model:   deps.Model,
	}
}

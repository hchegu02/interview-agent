package main

import (
	"fmt"

	"interview-agent/internal/config"
	"interview-agent/internal/parser"
	"interview-agent/internal/questionbank"
)

func buildQuestionBankStore(deps appDeps) (questionbank.Store, error) {
	if deps.PGPool != nil {
		return questionbank.NewPGStore(deps.PGPool), nil
	}
	items, err := questionbank.LoadSeedFile("seeds/question_bank.json")
	if err != nil {
		return nil, err
	}
	return questionbank.NewMemoryStore(items), nil
}

func buildQuestionBankImportService(cfg *config.Config, deps appDeps, store questionbank.Store) (*questionbank.ImportService, error) {
	writer, ok := store.(questionbank.Writer)
	if !ok {
		return nil, fmt.Errorf("question bank store does not support writes")
	}
	var importStore questionbank.ImportStore
	if deps.PGPool != nil {
		importStore = questionbank.NewPGImportStore(deps.PGPool)
	} else {
		importStore = questionbank.NewMemoryImportStore()
	}
	model, _, err := buildChatModel(cfg)
	if err != nil {
		return nil, err
	}
	embedder, err := buildEmbedder(cfg)
	if err != nil {
		return nil, err
	}
	return questionbank.NewImportService(questionbank.ImportServiceDeps{
		Imports:  importStore,
		Writer:   writer,
		Parser:   parser.NewDispatcher(),
		Model:    model,
		Embedder: embedder,
		Spool:    questionbank.NewLocalImportSpool(cfg.Server.ImportSpoolDir),
		OwnerID:  hostnameOwnerID(),
	}), nil
}

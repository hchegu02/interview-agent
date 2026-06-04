package agentkit

func RegisterDefaultInterviewSkills(reg *SkillRegistry) error {
	specs := []SkillSpec{
		{Name: "jd.analyze", Version: "v1", Description: "extract job title, level, required skills and responsibilities", InputSummary: "JD text", OutputSummary: "job profile", Permission: PermissionReadOnly},
		{Name: "resume.parse", Version: "v1", Description: "extract candidate skills, projects and highlights", InputSummary: "resume text", OutputSummary: "candidate profile", Permission: PermissionReadOnly},
		{Name: "profile.match", Version: "v1", Description: "compare job requirements with candidate profile", InputSummary: "job profile and candidate profile", OutputSummary: "gap report", Permission: PermissionReadOnly},
		{Name: "question.retrieve", Version: "v1", Description: "retrieve candidate interview questions from question bank", InputSummary: "job profile, gap report and filters", OutputSummary: "candidate question pool and retrieval trace", Permission: PermissionReadOnly},
		{Name: "answer.evaluate", Version: "v1", Description: "score candidate answer against expected points and rubric", InputSummary: "question and answer", OutputSummary: "evaluation result", Permission: PermissionWriteSession},
		{Name: "report.generate", Version: "v1", Description: "generate final interview report and practice plan", InputSummary: "session rounds and working memory", OutputSummary: "report", Permission: PermissionWriteReport},
	}
	for _, spec := range specs {
		if err := reg.Register(spec); err != nil {
			return err
		}
	}
	return nil
}

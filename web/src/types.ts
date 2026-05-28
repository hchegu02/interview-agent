export type Mode = "practice" | "exam";

export type Draft = {
  resume_text: string;
  jd_text: string;
  question_bank_filter?: QuestionBankFilter;
  analysis?: ProfileAnalyzeResponse;
  updated_at: string;
};

export type QuestionBankFilter = {
  skill_categories?: string[];
  scenarios?: string[];
  difficulty_min?: number;
  difficulty_max?: number;
  tags?: string[];
};

export type ProfileAnalyzeResponse = {
  job_profile?: JobProfile;
  candidate_profile?: CandidateProfile;
  gap_report?: GapReport;
  profile_analysis?: ProfileAnalysis;
};

export type JobProfile = {
  title: string;
  level: string;
  key_skills: string[];
  must_have: string[];
  nice_to_have: string[];
  years_required: number;
  jd_raw_text: string;
};

export type CandidateProfile = {
  years: number;
  skills: string[];
  weak_skills?: string[];
  projects?: ResumeProject[];
  highlights?: string[];
  resume_raw_text: string;
};

export type ResumeProject = {
  name: string;
  role?: string;
  highlights?: string[];
  stack?: string[];
};

export type GapReport = {
  matched_skills: string[];
  missing_skills: string[];
  overlap_score: number;
  strategy: string;
  reason?: string;
};

export type ProfileAnalysis = {
  match_score: number;
  summary: string;
  years_gap: number;
  matched_requirements?: string[];
  missing_requirements?: string[];
  strengths?: string[];
  risk_points?: string[];
  resume_suggestions?: string[];
  question_focus?: string[];
  project_probe_plan?: ProjectProbePlan[];
};

export type ProjectProbePlan = {
  project_name: string;
  focus: string;
  evidence?: string;
  suggested_question: string;
};

export type Session = {
  session_id: string;
  user_id?: string;
  mode: Mode;
  status: string;
  phase: string;
  progress?: ProgressStep[];
  job_profile?: JobProfile;
  candidate_profile?: CandidateProfile;
  profile_analysis?: ProfileAnalysis;
  question_bank_filter?: QuestionBankFilter;
  question?: InterviewQuestion;
  rounds?: InterviewRound[];
  report?: Report;
  created_at: string;
  updated_at: string;
};

export type ProgressStep = {
  key: string;
  label: string;
  status: string;
};

export type InterviewQuestion = {
  id: string;
  content: string;
  tags?: string[];
  difficulty?: number;
  skill_category?: string;
  expected_points?: string[];
};

export type InterviewRound = {
  round_id: string;
  number: number;
  question: InterviewQuestion;
  answer?: string;
  follow_ups?: InterviewFollowUp[];
  feedback?: InterviewFeedback;
  completed: boolean;
};

export type InterviewFollowUp = {
  question: string;
  answer?: string;
  feedback?: InterviewFeedback;
};

export type InterviewFeedback = {
  score: number;
  hit_points?: string[];
  missed_points?: string[];
  suggestion?: string;
  expected_points?: string[];
};

export type Report = {
  session_id: string;
  overall_score: number;
  skill_breakdown: Record<string, number>;
  transcript_analysis?: TranscriptAnalysis;
  drill_plan?: DrillPlanItem[];
  highlights: string[];
  improvements: string[];
  next_steps: string[];
};

export type TranscriptAnalysis = {
  rounds_analyzed: number;
  average_answer_chars: number;
  dimensions: TranscriptDimension[];
  patterns?: string[];
};

export type TranscriptDimension = {
  name: string;
  score: number;
  evidence?: string[];
  advice?: string;
};

export type DrillPlanItem = {
  practice_order: number;
  skill: string;
  reason: string;
  target_score: number;
  recommended_question_ids?: string[];
  recommended_questions?: string[];
};

export type SessionSummary = {
  session_id: string;
  mode: Mode;
  status: string;
  updated_at: string;
};

export type QuestionBankItem = {
  id: string;
  content: string;
  tags?: string[];
  skill_category?: string;
  difficulty?: number;
  source?: string;
  scenario?: string;
  role_tags?: string[];
  locale?: string;
  status?: string;
  embedding_status?: string;
  embedding_model?: string;
  embedding_error?: string;
  expected_points?: string[];
  rubric?: Record<string, string>;
  sample_answer?: string;
  follow_up_hints?: string[];
};

export type QuestionFacets = {
  skill_categories: Record<string, number>;
  scenarios: Record<string, number>;
  tags: Record<string, number>;
  difficulties: Record<string, number>;
};

export type QuestionBankImportJob = {
  id: string;
  source_type: "question_bank" | "document";
  filename: string;
  status: string;
  total_chunks: number;
  total_items: number;
  valid_items: number;
  invalid_items: number;
  imported_items: number;
  error?: string;
  created_at: string;
  updated_at: string;
};

export type QuestionBankImportItem = {
  id: string;
  question_id: string;
  status: string;
  item: QuestionBankItem;
  errors?: string[];
};

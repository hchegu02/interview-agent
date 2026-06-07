package httpapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"interview-agent/internal/domain"
	"interview-agent/internal/graph"
)

func (s *InterviewService) Start(ctx context.Context, req startInterviewRequest) (*domain.Session, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("%w: interview runner not configured", graph.ErrInvalidConfig)
	}
	cleanedReq, err := cleanStartInterviewRequest(req)
	if err != nil {
		return nil, err
	}
	req = cleanedReq
	// Start 只负责创建会话和推进首轮 Graph。真正的面试状态都写进 domain.Session，
	// 这样内存存储、PG 存储和事件快照看到的是同一份数据结构。
	id := req.SessionID
	if id == "" {
		// 无外部 session_id 时使用本地递增 ID，主要服务 demo/test。
		// 生产入口可以传入外部生成的 ULID/UUID，避免多实例本地计数冲突。
		s.mu.Lock()
		s.nextID++
		id = fmt.Sprintf("sess-%d", s.nextID)
		s.mu.Unlock()
	}
	now := time.Now()
	sess := &domain.Session{
		ID:        id,
		UserID:    req.UserID,
		Mode:      normalizeInterviewMode(req.Mode),
		Status:    domain.StatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
		JobProfile: &domain.JobProfile{
			JDRawText: req.JDText,
		},
		CandProfile: &domain.CandidateProfile{
			ResumeRawText: req.ResumeText,
		},
		QuestionBankFilter: cloneQuestionBankFilter(req.QuestionBankFilter),
		WorkingMemory:      domain.NewWorkingMemory(),
	}
	// 长期记忆只作为 WorkingMemory 的初始化信号，不能覆盖本次 JD/简历原始输入。
	s.hydrateWorkingMemoryFromLongTermMemory(ctx, sess)
	leaseAcquired := false
	// 首轮 Invoke 也要拿 mutation lease：start 可能被重复点击或多实例接管，
	// lease 先挡住重复执行，最终写保护仍由 SessionStore/PG row_version 兜底。
	if err := s.acquireSessionLease(ctx, sess.ID); err != nil {
		s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
		return nil, err
	}
	// 分布式锁只在 coordinator 存在时生效。拿锁后如果后续任一步失败，必须释放，
	// 否则同一个 session 会被错误地卡成“正在处理”。
	if s.coordinator != nil {
		leaseAcquired = true
	}
	releaseOnFailure := func() {
		if leaseAcquired {
			// 使用 background 释放是有意的：即使用户请求 ctx 已取消，也要尽量清理 lease。
			_, _ = s.coordinator.ReleaseLease(context.Background(), sess.ID, s.ownerID)
		}
	}
	// Invoke 会从 setup 节点一路推进，直到首题生成并触发 suspension。
	// HTTP 层不关心中间节点，只拿最终 Session 状态返回给前端。
	if err := s.runner.Invoke(ctx, sess); err != nil {
		s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
		releaseOnFailure()
		return nil, err
	}

	if err := s.store.Save(ctx, sess); err != nil {
		s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
		releaseOnFailure()
		return nil, err
	}
	// saveSessionSnapshot 可能把 Graph 产生的快照写回 Session，所以需要再 Save 一次。
	// 这里宁可多一次持久化，也不要让恢复流程读到半截状态。
	if s.saveSessionSnapshot(ctx, sess) {
		if err := s.store.Save(ctx, sess); err != nil {
			s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
			releaseOnFailure()
			return nil, err
		}
	}
	if leaseAcquired && shouldReleaseMutationLease(sess) {
		_ = s.releaseSessionLease(ctx, sess.ID)
		leaseAcquired = false
	}
	// created 事件在持久化成功后发布，避免前端收到一个数据库里还不存在的 session。
	s.publishEvent(ctx, interviewEventSessionCreated, sess, "", "")
	return sess, nil
}

// cloneQuestionBankFilter 复制并收敛前端传来的题库范围。
//
// 这里不直接复用请求指针，避免后续修改 Session 过滤条件时影响原请求对象；
// 空字符串、非法难度和空 filter 会被清理掉，让下游 RAG 只处理有效约束。
func cloneQuestionBankFilter(filter *domain.QuestionBankFilter) *domain.QuestionBankFilter {
	if filter == nil {
		return nil
	}
	out := &domain.QuestionBankFilter{
		SkillCategories: compactInterviewStrings(filter.SkillCategories),
		Scenarios:       compactInterviewStrings(filter.Scenarios),
		DifficultyMin:   normalizeScopeDifficulty(filter.DifficultyMin),
		DifficultyMax:   normalizeScopeDifficulty(filter.DifficultyMax),
		Tags:            compactInterviewStrings(filter.Tags),
	}
	if out.DifficultyMin > 0 && out.DifficultyMax > 0 && out.DifficultyMin > out.DifficultyMax {
		out.DifficultyMin, out.DifficultyMax = out.DifficultyMax, out.DifficultyMin
	}
	if len(out.SkillCategories) == 0 && len(out.Scenarios) == 0 && out.DifficultyMin == 0 && out.DifficultyMax == 0 && len(out.Tags) == 0 {
		return nil
	}
	return out
}

func compactInterviewStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func normalizeScopeDifficulty(n int) int {
	if n < 1 || n > 5 {
		return 0
	}
	return n
}

func (s *InterviewService) Answer(ctx context.Context, req answerInterviewRequest) (*domain.Session, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("%w: interview runner not configured", graph.ErrInvalidConfig)
	}

	// Answer 是“取会话 -> 填答案 -> Resume Graph -> 保存”的窄通道。
	// 不在 handler 里改 Session，是为了保证所有入口都走同一套并发和持久化规则。
	sess, err := s.getSessionForMutation(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	if req.UserID != "" && sess.UserID != req.UserID {
		return nil, fmt.Errorf("session %q not found", req.SessionID)
	}
	// Answer 是长耗时 mutation：续租成功后才写答案和 Resume，避免两个实例并发推进同一轮。
	if err := s.renewSessionLease(ctx, sess.ID); err != nil {
		return nil, err
	}
	// 先把用户输入写进当前等待点，再 Resume。Graph 节点只从 Session 读取答案，
	// 这样主问题回答和追问回答都走同一个状态入口。
	if err := fillPendingAnswer(sess, req.Answer); err != nil {
		return nil, err
	}
	if err := s.runner.Resume(ctx, sess); err != nil {
		s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
		return nil, err
	}
	// updated_at 只服务排序/展示；并发正确性由 PG row_version 负责。
	sess.UpdatedAt = nextUpdatedAt(sess.UpdatedAt)
	if err := s.store.Save(ctx, sess); err != nil {
		s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
		return nil, err
	}
	if s.saveSessionSnapshot(ctx, sess) {
		if err := s.store.Save(ctx, sess); err != nil {
			s.publishEvent(ctx, interviewEventSessionFailed, sess, "", err.Error())
			return nil, err
		}
	}
	// 长期记忆只在完整面试完成后持久化，避免半截评分污染用户画像。
	if sess.Status == domain.StatusCompleted {
		_ = s.persistLongTermMemory(ctx, sess)
	}
	if shouldReleaseMutationLease(sess) {
		_ = s.releaseSessionLease(ctx, sess.ID)
	}
	eventType := interviewEventSessionUpdated
	if sess.Status == domain.StatusCompleted {
		eventType = interviewEventSessionCompleted
	}
	s.publishEvent(ctx, eventType, sess, "", "")
	return sess, nil
}

// nextUpdatedAt 保证同一 Session 的更新时间单调递增。
//
// 这不是并发控制手段；它只避免时钟精度/回拨导致历史会话排序抖动。
func nextUpdatedAt(prev time.Time) time.Time {
	now := time.Now()
	if now.After(prev) {
		return now
	}
	return prev.Add(time.Nanosecond)
}

// fillPendingAnswer 根据当前暂停节点决定答案写入位置。
//
// pick_next 表示正在等主问题回答；probe_ask 表示正在等追问回答。
// 这里集中判断可以避免 handler 根据前端状态猜测要写哪个字段。
func fillPendingAnswer(sess *domain.Session, answer string) error {
	// Suspension 是新恢复语义；CurrentNode 只作为旧 Session 兼容回退。
	node, err := answerAwaitingNode(sess)
	if err != nil {
		return err
	}
	switch node {
	case "pick_next":
		round := sess.CurrentRound()
		if round == nil {
			return fmt.Errorf("%w: no current round for answer", ErrInvalidSessionState)
		}
		round.Answer = answer
		return nil
	case "probe_ask":
		round := sess.CurrentRound()
		if round == nil || len(round.FollowUps) == 0 {
			return fmt.Errorf("%w: no current follow-up for answer", ErrInvalidSessionState)
		}
		round.FollowUps[len(round.FollowUps)-1].Answer = answer
		return nil
	default:
		return fmt.Errorf("%w: session %q is not waiting for answer at node %q", ErrInvalidSessionState, sess.ID, node)
	}
}

// answerAwaitingNode 优先使用 Suspension 里的暂停节点。
//
// CurrentNode 是旧 Session 的兼容回退；新状态应以 Suspension.Node/Awaiting 为准。
func answerAwaitingNode(sess *domain.Session) (string, error) {
	if sess == nil {
		return "", fmt.Errorf("%w: nil session", ErrInvalidSessionState)
	}
	if sess.Suspension != nil {
		if sess.Suspension.Awaiting != "" && sess.Suspension.Awaiting != domain.SuspensionAwaitingAnswer {
			return "", fmt.Errorf("%w: session %q is awaiting %q", ErrInvalidSessionState, sess.ID, sess.Suspension.Awaiting)
		}
		if sess.Suspension.Node != "" {
			return sess.Suspension.Node, nil
		}
	}
	return sess.CurrentNode, nil
}

// shouldReleaseMutationLease 判断本轮 mutation 是否可以释放 lease。
//
// Graph 已完成或进入 suspension 时，当前 HTTP mutation 已经结束；
// 继续持有 lease 只会阻塞下一次 answer 或实例接管。
func shouldReleaseMutationLease(sess *domain.Session) bool {
	if sess == nil {
		return false
	}
	return sess.Status == domain.StatusCompleted || sess.Suspension != nil
}

// publishEvent 是 InterviewService 到 SSE/事件总线的唯一出口。
//
// 调用方只传内部事件类型和 Session，公开字段过滤在事件构造/响应构造里完成。
func (s *InterviewService) publishEvent(ctx context.Context, eventType string, sess *domain.Session, node, errMsg string) {
	if s == nil || s.events == nil {
		return
	}
	s.events.Publish(ctx, buildInterviewEventWithContext(ctx, eventType, sess, node, errMsg))
}

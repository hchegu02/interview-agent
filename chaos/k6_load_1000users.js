import http from "k6/http";
import { check, sleep } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";

const BASE_URL = __ENV.BASE_URL || "http://127.0.0.1:8080";
const USER_PREFIX = __ENV.K6_USER_PREFIX || "k6";
const THINK_TIME_SECONDS = Number(__ENV.K6_THINK_TIME_SECONDS || "1");
const SSE_TIMEOUT = __ENV.K6_SSE_TIMEOUT || "10s";
const ANSWER_RETRIES = Number(__ENV.K6_ANSWER_RETRIES || "5");
const ANSWER_RETRY_SLEEP_SECONDS = Number(__ENV.K6_ANSWER_RETRY_SLEEP_SECONDS || "1");

export const http503 = new Counter("interview_http_503_total");
export const http409 = new Counter("interview_http_409_total");
export const http503Rate = new Rate("interview_http_503_rate");
export const http409Rate = new Rate("interview_http_409_rate");
export const sseFirstPacket = new Trend("interview_sse_first_packet_ms", true);
export const sessionsStarted = new Counter("interview_sessions_started_total");
export const answersCompleted = new Counter("interview_answers_completed_total");
export const sseConnected = new Counter("interview_sse_connected_total");

export const options = {
  scenarios: {
    interview_1000_users: {
      executor: "ramping-vus",
      stages: [
        { duration: __ENV.K6_RAMP_UP || "5m", target: Number(__ENV.K6_TARGET_USERS || "1000") },
        { duration: __ENV.K6_HOLD || "10m", target: Number(__ENV.K6_TARGET_USERS || "1000") },
        { duration: __ENV.K6_RAMP_DOWN || "2m", target: 0 },
      ],
      gracefulRampDown: "30s",
    },
  },
  thresholds: {
    interview_http_503_rate: ["rate<0.05"],
    interview_http_409_rate: ["rate<0.50"],
    interview_sse_first_packet_ms: ["p(95)<2000"],
  },
};

function jsonHeaders() {
  return {
    headers: { "Content-Type": "application/json" },
    responseCallback: http.expectedStatuses(200, 409, 503),
  };
}

function allowedStatuses() {
  return { responseCallback: http.expectedStatuses(200, 409, 503) };
}

function recordStatus(res, endpoint) {
  const tags = { endpoint };
  const is503 = res && res.status === 503;
  const is409 = res && res.status === 409;

  if (is503) {
    http503.add(1, tags);
  }
  if (is409) {
    http409.add(1, tags);
  }
  http503Rate.add(is503, tags);
  http409Rate.add(is409, tags);
}

function parseJson(res) {
  if (!res || !res.body) {
    return {};
  }
  try {
    return res.json();
  } catch (_) {
    return {};
  }
}

export default function () {
  const userID = `${USER_PREFIX}-u-${__VU}`;
  const sessionID = `${USER_PREFIX}-s-${__VU}-${__ITER}-${Date.now()}`;

  const startPayload = JSON.stringify({
    session_id: sessionID,
    user_id: userID,
    jd_text: "需要 Go 后端工程师，熟悉 Redis、Postgres、SSE 和高并发服务治理。",
    resume_text: "候选人有 Go 后端经验，做过 Redis 缓存、Postgres 存储和 HTTP 服务。",
  });

  const startRes = http.post(`${BASE_URL}/api/interview/start`, startPayload, {
    ...jsonHeaders(),
    tags: { endpoint: "interview_start" },
  });
  recordStatus(startRes, "interview_start");

  const started = check(startRes, {
    "start returns 200/409/503": (r) => [200, 409, 503].includes(r.status),
  });
  if (!started || startRes.status !== 200) {
    sleep(THINK_TIME_SECONDS);
    return;
  }
  sessionsStarted.add(1, { endpoint: "interview_start" });

  const startBody = parseJson(startRes);
  const effectiveSessionID = startBody.session_id || sessionID;
  const streamUrl = `${BASE_URL}/api/interview/stream?session_id=${encodeURIComponent(effectiveSessionID)}&user_id=${encodeURIComponent(userID)}`;
  const streamRes = http.get(streamUrl, {
    ...allowedStatuses(),
    responseType: "none",
    timeout: SSE_TIMEOUT,
    tags: { endpoint: "interview_stream" },
  });
  recordStatus(streamRes, "interview_stream");

  check(streamRes, {
    "stream returns 200/409/503": (r) => [200, 409, 503].includes(r.status),
  });
  if (streamRes && streamRes.status === 200 && streamRes.timings && streamRes.timings.waiting >= 0) {
    sseConnected.add(1, { endpoint: "interview_stream" });
    sseFirstPacket.add(streamRes.timings.waiting, { endpoint: "interview_stream" });
  }

  const answerPayload = JSON.stringify({
    session_id: effectiveSessionID,
    user_id: userID,
    answer: "G 是 goroutine，M 是系统线程，P 维护本地运行队列并参与调度。",
  });

  let answerRes = null;
  for (let attempt = 1; attempt <= ANSWER_RETRIES; attempt++) {
    answerRes = http.post(`${BASE_URL}/api/interview/answer`, answerPayload, {
      ...jsonHeaders(),
      tags: { endpoint: "interview_answer" },
    });
    recordStatus(answerRes, "interview_answer");
    if (answerRes && answerRes.status !== 409) {
      break;
    }
    sleep(ANSWER_RETRY_SLEEP_SECONDS);
  }

  check(answerRes, {
    "answer returns 200/409/503": (r) => r && [200, 409, 503].includes(r.status),
  });
  if (answerRes && answerRes.status === 200) {
    answersCompleted.add(1, { endpoint: "interview_answer" });
  }

  sleep(THINK_TIME_SECONDS);
}

function metricValue(data, name, field, fallback = null) {
  const metric = data.metrics && data.metrics[name];
  if (!metric || !metric.values || metric.values[field] === undefined) {
    return fallback;
  }
  return metric.values[field];
}

function thresholdFailures(data) {
  const failures = [];
  for (const [metricName, metric] of Object.entries(data.metrics || {})) {
    for (const [thresholdName, threshold] of Object.entries(metric.thresholds || {})) {
      if (threshold && threshold.ok === false) {
        failures.push(`${metricName}: ${thresholdName}`);
      }
    }
  }
  return failures;
}

function checkResults(data) {
  const started = metricValue(data, "interview_sessions_started_total", "count", 0);
  const completed = metricValue(data, "interview_answers_completed_total", "count", 0);
  return [
    {
      name: "http_transport_error_rate",
      ok: true,
      value: metricValue(data, "http_req_failed", "rate", null),
      threshold: "reported_only",
    },
    {
      name: "http_503_rate",
      ok: metricValue(data, "interview_http_503_rate", "rate", 1) < 0.05,
      value: metricValue(data, "interview_http_503_rate", "rate", null),
      threshold: "rate<0.05",
    },
    {
      name: "http_409_rate",
      ok: metricValue(data, "interview_http_409_rate", "rate", 1) < 0.50,
      value: metricValue(data, "interview_http_409_rate", "rate", null),
      threshold: "rate<0.50",
    },
    {
      name: "sse_first_packet_p95_ms",
      ok: metricValue(data, "interview_sse_first_packet_ms", "p(95)", 0) < 2000,
      value: metricValue(data, "interview_sse_first_packet_ms", "p(95)", null),
      threshold: "p(95)<2000",
    },
    {
      name: "sessions_started",
      ok: started > 0,
      value: started,
      threshold: ">0",
    },
    {
      name: "answers_completed",
      ok: completed > 0,
      value: completed,
      threshold: ">0",
    },
  ];
}

export function handleSummary(data) {
  const failures = thresholdFailures(data);
  const checks = checkResults(data);
  for (const checkItem of checks) {
    if (!checkItem.ok) {
      failures.push(checkItem.name);
    }
  }

  const summary = {
    kind: "k6_load_1000users",
    status: failures.length === 0 ? "pass" : "fail",
    base_url: BASE_URL,
    started_at: new Date().toISOString(),
    duration_ms: data.state && data.state.testRunDurationMs ? data.state.testRunDurationMs : null,
    recovery_ms: null,
    checks,
    failures,
    sessions_started: metricValue(data, "interview_sessions_started_total", "count", 0),
    answers_completed: metricValue(data, "interview_answers_completed_total", "count", 0),
    sse_reconnect_ok: null,
    metrics: {
      http_req_duration_p95_ms: metricValue(data, "http_req_duration", "p(95)", null),
      http_req_failed_rate: metricValue(data, "http_req_failed", "rate", null),
      http_503_rate: metricValue(data, "interview_http_503_rate", "rate", null),
      http_409_rate: metricValue(data, "interview_http_409_rate", "rate", null),
      sse_first_packet_p95_ms: metricValue(data, "interview_sse_first_packet_ms", "p(95)", null),
    },
  };

  const body = JSON.stringify(summary, null, 2);
  const outputs = { stdout: `${body}\n` };
  if (__ENV.K6_SUMMARY_PATH) {
    outputs[__ENV.K6_SUMMARY_PATH] = body;
  }
  return outputs;
}

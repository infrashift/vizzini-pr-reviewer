-- Vizzini cost attribution over LiteLLM's spend logs (Vizzini ISSUE-025,
-- ADR-0015). LiteLLM writes one row per request to "LiteLLM_SpendLogs";
-- Vizzini's attribution arrives in metadata->'spend_logs_metadata' (from the
-- x-litellm-spend-logs-metadata header) and the requester as end_user.
-- Apply with: psql "$DATABASE_URL" -f deploy/litellm/views.sql (idempotent).

-- One row per Vizzini model request.
CREATE OR REPLACE VIEW vizzini_requests AS
SELECT
    s.request_id,
    s."startTime"                                          AS started_at,
    s.model,
    s.status,
    s.spend                                                AS cost_usd,
    s.prompt_tokens                                        AS input_tokens,
    s.completion_tokens                                    AS output_tokens,
    COALESCE((s.metadata -> 'usage_object' -> 'prompt_tokens_details' ->> 'cached_tokens')::int, 0)
                                                           AS cached_input_tokens,
    NULLIF(s.end_user, '')                                 AS requester,
    s.team_id,
    m ->> 'vizzini.run_id'                                 AS run_id,
    m ->> 'vizzini.pr_key'                                 AS pr_key,
    m ->> 'vizzini.iteration'                              AS iteration,
    m ->> 'vizzini.agent'                                  AS agent,
    m ->> 'vizzini.mode'                                   AS mode,
    m ->> 'vizzini.chunk'                                  AS chunk,
    COALESCE((m ->> 'vizzini.attempt')::int, 0)            AS attempt,
    COALESCE((m ->> 'vizzini.repair')::int, 0)             AS repair
FROM "LiteLLM_SpendLogs" s
CROSS JOIN LATERAL (SELECT s.metadata -> 'spend_logs_metadata' AS m) md
WHERE m ? 'vizzini.run_id';

-- Facts a run knows and LiteLLM does not (from the review report), written
-- by whatever runs Vizzini (CI, a collector; vizzini-uat's record_runs.py);
-- the source of truth stays the review report itself.
CREATE TABLE IF NOT EXISTS vizzini_run_facts (
    run_id        text PRIMARY KEY,
    pr_key        text,
    iteration     text,
    agent         text,
    model         text,
    fixture       text,
    findings      int,
    blocking      int,     -- Critical + Major findings
    gate_exit     int,     -- pr-review exit code: 0 pass, 2 blocked
    risk_score    int,
    recorded_at   timestamptz NOT NULL DEFAULT now()
);

-- One row per run: what it cost, what it was for, what it found.
CREATE OR REPLACE VIEW vizzini_runs AS
SELECT
    r.run_id,
    r.pr_key,
    r.iteration,
    r.agent,
    r.requester,
    string_agg(DISTINCT r.mode, ',')                       AS modes,
    string_agg(DISTINCT r.model, ',')                      AS models,
    count(*)                                               AS requests,
    sum(r.input_tokens)                                    AS input_tokens,
    sum(r.output_tokens)                                   AS output_tokens,
    sum(r.cached_input_tokens)                             AS cached_input_tokens,
    round(sum(r.cost_usd)::numeric, 6)                     AS cost_usd,
    round(COALESCE(sum(r.cost_usd) FILTER (WHERE r.repair > 0 OR r.attempt > 0), 0)::numeric, 6)
                                                           AS wasted_on_repairs_and_retries_usd,
    min(r.started_at)                                      AS started_at,
    f.findings,
    f.blocking,
    CASE WHEN f.findings > 0 THEN round((sum(r.cost_usd) / f.findings)::numeric, 6) END
                                                           AS cost_per_finding_usd
FROM vizzini_requests r
LEFT JOIN vizzini_run_facts f USING (run_id)
GROUP BY r.run_id, r.pr_key, r.iteration, r.agent, r.requester, f.findings, f.blocking;

-- One row per pull request across its lifecycle: every run on every head.
CREATE OR REPLACE VIEW vizzini_pr_costs AS
SELECT
    pr_key,
    count(DISTINCT iteration)                              AS iterations,
    count(DISTINCT run_id)                                 AS runs,
    count(*)                                               AS requests,
    string_agg(DISTINCT model, ',')                        AS models,
    string_agg(DISTINCT requester, ',')                    AS requesters,
    sum(input_tokens)                                      AS input_tokens,
    sum(output_tokens)                                     AS output_tokens,
    round(sum(cost_usd)::numeric, 6)                       AS cost_usd,
    min(started_at)                                        AS first_run,
    max(started_at)                                        AS last_run
FROM vizzini_requests
WHERE pr_key IS NOT NULL
GROUP BY pr_key;

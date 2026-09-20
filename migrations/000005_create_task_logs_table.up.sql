CREATE TABLE IF NOT EXISTS task_logs (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    changed_by UUID NOT NULL REFERENCES users(id),
    old_assignee_id UUID REFERENCES users(id),
    new_assignee_id UUID REFERENCES users(id),
    action VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_task_logs_task_id ON task_logs(task_id);

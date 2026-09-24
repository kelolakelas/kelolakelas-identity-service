CREATE TABLE platform_admin_assignments (
    user_id uuid PRIMARY KEY REFERENCES users(id),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now()
);

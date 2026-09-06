-- Add role and password fields to users table for admin authentication
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS role VARCHAR(20) NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
ADD COLUMN IF NOT EXISTS password VARCHAR(255);

-- Create index on role for faster admin queries
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);

-- Update existing users to have 'user' role (if any exist)
UPDATE users SET role = 'user' WHERE role IS NULL;


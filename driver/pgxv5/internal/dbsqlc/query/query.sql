-- name: AcquireLeadership :one
INSERT INTO leaders(name, leader_id, elected_at, expires_at)
VALUES(@name, @leaderId, NOW(), NOW() + MAKE_INTERVAL(secs => @leaseDuration))
ON CONFLICT (name) DO UPDATE
SET elected_at = NOW(),
    expires_at = NOW() + MAKE_INTERVAL(secs => @leaseDuration),
    renewed_at = NOW(),
    leader_id = @leaderId
WHERE expires_at < NOW() AND name = @name
RETURNING *;

-- name: LeaderRenewal :one
UPDATE leaders
SET renewed_at = NOW(),
    expires_at = NOW() + MAKE_INTERVAL(secs => @leaseDuration)
WHERE name = @name AND leader_id = @leaderId and expires_at >= NOW()
RETURNING *;

-- name: ReleaseLeadership :exec
DELETE FROM leaders
WHERE name = @name AND leader_id = @leaderId;

-- name: ResignLeadership :exec
UPDATE leaders
SET expires_at = NOW() - MAKE_INTERVAL(secs => 1))
WHERE name = @name AND leader_id = @leaderId;
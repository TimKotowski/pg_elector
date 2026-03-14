-- name: AcquireLeadership :execrows
INSERT INTO leaders(name, leader_id, elected_at, expires_at)
VALUES(@name, @leaderId, now(), MAKE_INTERVAL(secs => @leaseDuration)) ON CONFLICT (name) DO UPDATE
SET elected_at = NOW(),
    expires_at = NOW() + MAKE_INTERVAL(secs => @leaseDuration),
    renewed_at = NULL,
    term = leaders.term + 1,
    leader_id = @leaderId
WHERE expires_at < NOW() AND name = @name;

-- name: LeaderRenewal :execrows
UPDATE leaders
SET renewed_at = NOW(),
    expires_at = NOW() + MAKE_INTERVAL(secs => @leaseDuration)
WHERE name = @name AND leader_id = @leaderId;

-- name: ReleaseLeadership :execrows
DELETE FROM leaders
WHERE name = @name AND leader_id = @leaderId;
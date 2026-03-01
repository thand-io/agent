package domain

import "time"

// IsCompletedGrantState reports whether the persisted grant has already been
// transitioned into its completed/tombstone state.
func IsCompletedGrantState(grant GrantState) bool {
	return !grant.CompletedAtWallUTC.IsZero()
}

// IsActiveGrantState reports whether the persisted grant is still active and
// should block a new grant for the same user.
func IsActiveGrantState(grant GrantState, nowMonoNS int64, nowWallUTC time.Time) bool {
	return !IsCompletedGrantState(grant) && !IsExpiredGrantState(grant, nowMonoNS, nowWallUTC)
}

// IsExpiredGrantState reports whether a persisted grant has expired according to
// the helper's fail-secure wall-clock and monotonic-clock rules.
func IsExpiredGrantState(grant GrantState, nowMonoNS int64, nowWallUTC time.Time) bool {
	switch {
	case grant.DurationSeconds <= 0:
		return true
	case grant.GrantedAtMonoNS <= 0:
		return true
	case grant.GrantedAtWallUTC.IsZero():
		return true
	case nowWallUTC.IsZero():
		return true
	}

	durationNS := grant.DurationSeconds * int64(time.Second)

	wallExpiry := grant.GrantedAtWallUTC.Add(time.Duration(durationNS))
	if wallExpiry.Before(nowWallUTC) {
		return true
	}

	isMonoValid := nowMonoNS >= grant.GrantedAtMonoNS
	if !isMonoValid {
		return false
	}

	return nowMonoNS-grant.GrantedAtMonoNS >= durationNS
}

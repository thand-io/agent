package handler

import (
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

func isActiveGrantState(grant domain.GrantState, nowMonoNS int64, nowWallUTC time.Time) bool {
	return !isCompletedGrantState(grant) && !isExpiredGrantState(grant, nowMonoNS, nowWallUTC)
}

func isExpiredGrantState(grant domain.GrantState, nowMonoNS int64, nowWallUTC time.Time) bool {
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

package common

import "net/url"

func IsValidLoginServer(hostname string) bool {

	// paarse url
	_, err := url.Parse(hostname)

	return err == nil
}

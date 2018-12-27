package types

import "strings"

const routableSeparator = "-"

type RoutableAddress string

func NewRoutableAddress(from, to string) RoutableAddress {
	return RoutableAddress(from + routableSeparator + to)
}

func (ra RoutableAddress) From() string {
	return strings.Split(string(ra), routableSeparator)[0]
}

func (ra RoutableAddress) To() string {
	return strings.Split(string(ra), routableSeparator)[1]
}

func (ra RoutableAddress) Swap() RoutableAddress {
	split := strings.Split(string(ra), routableSeparator)
	return RoutableAddress(split[1] + routableSeparator + split[0])
}

func (ra RoutableAddress) String() string {
	return string(ra)
}

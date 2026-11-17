package ui

import "audit-loss/internal/sink"

func judgeAllMain(st sink.Stats) (bool, string) {
	if st.Sent <= 0 || st.Received != st.Sent || st.ByCollector["main"] != st.Sent || st.ByCollector["secondary"] != 0 || len(st.Missing) != 0 {
		return false, counts(st)
	}
	return true, counts(st)
}

func judgeAllSecondary(st sink.Stats) (bool, string) {
	if st.Sent <= 0 || st.Received != st.Sent || st.ByCollector["secondary"] != st.Sent || st.ByCollector["main"] != 0 || len(st.Missing) != 0 {
		return false, counts(st)
	}
	return true, counts(st)
}

func judgeArrived(st sink.Stats) (bool, string) {
	if st.Sent <= 0 || st.Received != st.Sent || len(st.Missing) != 0 {
		return false, counts(st)
	}
	return true, counts(st)
}

func judgeSlow(st sink.Stats) (bool, string) {
	if st.Sent <= 0 || st.Received != st.Sent || len(st.Missing) != 0 || st.LatencyMS.Max < 100 {
		return false, counts(st)
	}
	return true, counts(st)
}

func judgeStall(st sink.Stats) (bool, string) {
	if st.Sent <= 0 || st.Received != st.Sent || st.Duplicates < 1 {
		return false, counts(st)
	}
	return true, counts(st)
}

func judgeMalformed(st sink.Stats) (bool, string) {
	if st.Received <= 0 || st.Received >= st.Sent || st.Rejected <= 0 {
		return false, counts(st)
	}
	return true, counts(st)
}

func judgeBadSignature(st sink.Stats) (bool, string) {
	if st.VerifyFailures <= 0 || st.Received >= st.Sent {
		return false, counts(st)
	}
	return true, counts(st)
}

func counts(st sink.Stats) string {
	return "sent " + itoa(st.Sent) +
		", received " + itoa(st.Received) +
		", missing " + itoa(len(st.Missing)) +
		", duplicates " + itoa(st.Duplicates) +
		", main " + itoa(st.ByCollector["main"]) +
		", secondary " + itoa(st.ByCollector["secondary"]) +
		", rejected " + itoa(st.Rejected) +
		", verify failures " + itoa(st.VerifyFailures)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

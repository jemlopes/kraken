package main

type broadcastHeap []string

func (h *broadcastHeap) Push(x interface{}) {
	*h = append(*h, x.(string))
}

func (h *broadcastHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (h *broadcastHeap) PopMultiple(q int) []string {
	ret := []string{}
	old := *h
	n := len(old)
	max := q
	i := q - 1
	ct := 0
	retCount := 0
	found := false
	if n < q {
		max = n
	}
	for {
		oldid := old[i]
		found = false
		retCount = len(ret)
		for j := 0; j < retCount; j++ {
			if ret[j] == oldid {
				found = true
			}
		}
		if !found {
			ret = append(ret, old[i])
			ct++
		}
		i--
		if ct >= max || i < 0 {
			break
		}
	}

	for i := q - 1; i >= 0; i-- {
		oldid := old[i]
		found = false
		retCount = len(ret)
		for j := 0; j < retCount; j++ {
			if ret[j] == oldid {
				found = true
			}
		}
		if !found {
			ret = append(ret, old[i])
		}

	}
	*h = old[0 : n-q]
	return ret
}

func (h broadcastHeap) Len() int           { return len(h) }
func (h broadcastHeap) Less(i, j int) bool { return false }
func (h broadcastHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

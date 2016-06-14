package time

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func validateResult(t *testing.T, tr Ranges, expected []Range) {
	l := tr.(*ranges).sortedRanges
	require.Equal(t, len(expected), l.Len())
	idx := 0
	for e := l.Front(); e != nil; e = e.Next() {
		require.Equal(t, e.Value.(Range), expected[idx])
		idx++
	}
}

func validateIter(t *testing.T, it RangeIter, expected []Range) {
	idx := 0
	for it.Next() {
		r := it.Value()
		require.Equal(t, expected[idx], r)
		idx++
	}
}

func getTypedTimeRanges() *ranges {
	return NewRanges().(*ranges)
}

func getRangesToAdd() []Range {
	return []Range{
		{Start: testStart, End: testStart.Add(time.Second)},
		{Start: testStart.Add(10 * time.Second), End: testStart.Add(15 * time.Second)},
		{Start: testStart.Add(-3 * time.Second), End: testStart.Add(-1 * time.Second)},
		{Start: testStart.Add(-8 * time.Second), End: testStart.Add(-5 * time.Second)},
		{Start: testStart.Add(-1 * time.Second), End: testStart},
		{Start: testStart.Add(time.Second), End: testStart.Add(8 * time.Second)},
		{Start: testStart.Add(-10 * time.Second), End: testStart.Add(12 * time.Second)},
	}
}

func getRangesToRemove() []Range {
	return []Range{
		{Start: testStart.Add(-5 * time.Second), End: testStart.Add(-3 * time.Second)},
		{Start: testStart.Add(-6 * time.Second), End: testStart.Add(8 * time.Second)},
		{Start: testStart.Add(12 * time.Second), End: testStart.Add(13 * time.Second)},
		{Start: testStart.Add(10 * time.Second), End: testStart.Add(12 * time.Second)},
	}
}

func getPopulatedRanges(ranges []Range, start, end int) Ranges {
	tr := NewRanges()
	for _, r := range ranges[start:end] {
		tr = tr.AddRange(r)
	}
	return tr
}

func TestIsEmpty(t *testing.T) {
	var tr *ranges
	require.True(t, tr.IsEmpty())

	tr = getTypedTimeRanges()
	require.True(t, tr.IsEmpty())

	tr.sortedRanges.PushBack(Range{})
	require.False(t, tr.IsEmpty())

}

func TestClone(t *testing.T) {
	rangesToAdd := getRangesToAdd()
	tr := getPopulatedRanges(rangesToAdd, 0, 4)

	expectedResults := []Range{rangesToAdd[3], rangesToAdd[2], rangesToAdd[0], rangesToAdd[1]}
	validateResult(t, tr, expectedResults)

	cloned := tr.(*ranges).clone()
	tr = tr.RemoveRange(rangesToAdd[0])
	validateResult(t, cloned, expectedResults)
	validateResult(t, tr, []Range{rangesToAdd[3], rangesToAdd[2], rangesToAdd[1]})
}

func TestAddRange(t *testing.T) {
	tr := NewRanges()
	tr = tr.AddRange(Range{})
	validateResult(t, tr, []Range{})

	rangestoAdd := getRangesToAdd()
	expectedResults := [][]Range{
		{rangestoAdd[0]},
		{rangestoAdd[0], rangestoAdd[1]},
		{rangestoAdd[2], rangestoAdd[0], rangestoAdd[1]},
		{rangestoAdd[3], rangestoAdd[2], rangestoAdd[0], rangestoAdd[1]},
		{rangestoAdd[3], {Start: testStart.Add(-3 * time.Second), End: testStart.Add(time.Second)}, rangestoAdd[1]},
		{rangestoAdd[3], {Start: testStart.Add(-3 * time.Second), End: testStart.Add(8 * time.Second)}, rangestoAdd[1]},
		{{Start: testStart.Add(-10 * time.Second), End: testStart.Add(15 * time.Second)}},
	}

	saved := tr
	for i, r := range rangestoAdd {
		tr = tr.AddRange(r)
		validateResult(t, tr, expectedResults[i])
	}
	validateResult(t, saved, []Range{})
}

func TestAddRanges(t *testing.T) {
	rangesToAdd := getRangesToAdd()

	tr := getPopulatedRanges(rangesToAdd, 0, 4)
	tr = tr.AddRanges(nil)

	expectedResults := []Range{rangesToAdd[3], rangesToAdd[2], rangesToAdd[0], rangesToAdd[1]}
	validateResult(t, tr, expectedResults)

	tr2 := getPopulatedRanges(rangesToAdd, 4, 7)
	saved := tr
	tr = tr.AddRanges(tr2)

	expectedResults2 := []Range{{Start: testStart.Add(-10 * time.Second), End: testStart.Add(15 * time.Second)}}
	validateResult(t, tr, expectedResults2)
	validateResult(t, saved, expectedResults)
}

func TestRemoveRange(t *testing.T) {
	tr := getPopulatedRanges(getRangesToAdd(), 0, 4)

	rangesToRemove := getRangesToRemove()
	expectedResults := [][]Range{
		{
			{Start: testStart.Add(-8 * time.Second), End: testStart.Add(-5 * time.Second)},
			{Start: testStart.Add(-3 * time.Second), End: testStart.Add(-1 * time.Second)},
			{Start: testStart, End: testStart.Add(time.Second)},
			{Start: testStart.Add(10 * time.Second), End: testStart.Add(15 * time.Second)},
		},
		{
			{Start: testStart.Add(-8 * time.Second), End: testStart.Add(-6 * time.Second)},
			{Start: testStart.Add(10 * time.Second), End: testStart.Add(15 * time.Second)},
		},
		{
			{Start: testStart.Add(-8 * time.Second), End: testStart.Add(-6 * time.Second)},
			{Start: testStart.Add(10 * time.Second), End: testStart.Add(12 * time.Second)},
			{Start: testStart.Add(13 * time.Second), End: testStart.Add(15 * time.Second)},
		},
		{
			{Start: testStart.Add(-8 * time.Second), End: testStart.Add(-6 * time.Second)},
			{Start: testStart.Add(13 * time.Second), End: testStart.Add(15 * time.Second)},
		},
	}

	saved := tr
	for i, r := range rangesToRemove {
		tr = tr.RemoveRange(r)
		validateResult(t, tr, expectedResults[i])
	}

	tr = tr.RemoveRange(Range{})
	validateResult(t, tr, expectedResults[3])

	tr = tr.RemoveRange(Range{
		Start: testStart.Add(-10 * time.Second),
		End:   testStart.Add(15 * time.Second),
	})
	require.True(t, tr.IsEmpty())
	validateResult(t, saved, expectedResults[0])
}

func TestRemoveRanges(t *testing.T) {
	tr := getPopulatedRanges(getRangesToAdd(), 0, 4)
	tr = tr.RemoveRanges(nil)

	expectedResults := []Range{
		{Start: testStart.Add(-8 * time.Second), End: testStart.Add(-5 * time.Second)},
		{Start: testStart.Add(-3 * time.Second), End: testStart.Add(-1 * time.Second)},
		{Start: testStart, End: testStart.Add(time.Second)},
		{Start: testStart.Add(10 * time.Second), End: testStart.Add(15 * time.Second)},
	}
	validateResult(t, tr, expectedResults)

	saved := tr
	tr2 := getPopulatedRanges(getRangesToRemove(), 0, 4)
	tr = tr.RemoveRanges(tr2)

	expectedResults2 := []Range{
		{Start: testStart.Add(-8 * time.Second), End: testStart.Add(-6 * time.Second)},
		{Start: testStart.Add(13 * time.Second), End: testStart.Add(15 * time.Second)},
	}
	validateResult(t, tr, expectedResults2)
	validateResult(t, saved, expectedResults)
}

func TestContains(t *testing.T) {
	tr := getPopulatedRanges(getRangesToAdd(), 0, 4)
	require.True(t, tr.Contains(Range{Start: testStart, End: testStart}))
	require.True(t, tr.Contains(Range{Start: testStart, End: testStart.Add(time.Second)}))
	require.True(t, tr.Contains(Range{Start: testStart.Add(-7 * time.Second), End: testStart.Add(-5 * time.Second)}))
	require.False(t, tr.Contains(Range{Start: testStart.Add(-7 * time.Second), End: testStart.Add(-4 * time.Second)}))
	require.False(t, tr.Contains(Range{Start: testStart.Add(-3 * time.Second), End: testStart.Add(1 * time.Second)}))
	require.False(t, tr.Contains(Range{Start: testStart.Add(9 * time.Second), End: testStart.Add(15 * time.Second)}))
}

func TestIter(t *testing.T) {
	rangesToAdd := getRangesToAdd()
	tr := getPopulatedRanges(rangesToAdd, 0, 4)
	expectedResults := []Range{
		{Start: testStart.Add(-8 * time.Second), End: testStart.Add(-5 * time.Second)},
		{Start: testStart.Add(-3 * time.Second), End: testStart.Add(-1 * time.Second)},
		{Start: testStart, End: testStart.Add(time.Second)},
		{Start: testStart.Add(10 * time.Second), End: testStart.Add(15 * time.Second)},
	}
	validateIter(t, tr.Iter(), expectedResults)
	tr = tr.RemoveRange(rangesToAdd[2])
	validateIter(t, tr.Iter(), append(expectedResults[:1], expectedResults[2:]...))
}
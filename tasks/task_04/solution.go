package main

type Stats struct {
	Count         int
	Sum, Min, Max int64
}

func Calc(nums []int64) Stats {
	if len(nums) < 2 {
		return Stats{}
	}

	firstDiff := nums[1] - nums[0]
	stats := Stats{
		Count: 1,
		Sum:   firstDiff,
		Min:   firstDiff,
		Max:   firstDiff,
	}

	for index := 2; index < len(nums); index++ {
		difference := nums[index] - nums[index-1]
		stats.Count++
		stats.Sum += difference

		if difference < stats.Min {
			stats.Min = difference
		}
		if difference > stats.Max {
			stats.Max = difference
		}

	}
	return stats
}

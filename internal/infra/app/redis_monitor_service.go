package app

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type RedisCommandStat struct {
	Command string `json:"command"`
	Calls   int64  `json:"calls"`
	Usec    int64  `json:"usec"`
}

type RedisMonitorInfo struct {
	Info         map[string]string  `json:"info"`
	DBSize       int64              `json:"dbSize"`
	CommandStats []RedisCommandStat `json:"commandStats"`
}

type RedisMonitorService struct{ inspector RedisInspector }

func NewRedisMonitorService(inspector RedisInspector) *RedisMonitorService {
	return &RedisMonitorService{inspector: inspector}
}

func (s *RedisMonitorService) Monitor(ctx context.Context) (RedisMonitorInfo, error) {
	info, stats, size, err := s.inspector.Snapshot(ctx)
	if err != nil {
		return RedisMonitorInfo{}, fmt.Errorf("inspect Redis: %w", err)
	}
	result := RedisMonitorInfo{Info: redisProperties(info, "\n", ":"), DBSize: size, CommandStats: []RedisCommandStat{}}
	for key, raw := range redisProperties(stats, "\n", ":") {
		command, ok := strings.CutPrefix(key, "cmdstat_")
		if !ok || command == "" {
			continue
		}
		fields := redisProperties(raw, ",", "=")
		calls, err := strconv.ParseInt(fields["calls"], 10, 64)
		if err != nil || calls < 0 {
			continue
		}
		usec, _ := strconv.ParseInt(fields["usec"], 10, 64)
		result.CommandStats = append(result.CommandStats, RedisCommandStat{Command: command, Calls: calls, Usec: usec})
	}
	slices.SortFunc(result.CommandStats, func(a, b RedisCommandStat) int { return strings.Compare(a.Command, b.Command) })
	return result, nil
}

func redisProperties(raw, separator, delimiter string) map[string]string {
	props := make(map[string]string)
	for line := range strings.SplitSeq(raw, separator) {
		line = strings.TrimSpace(line)
		if key, value, ok := strings.Cut(line, delimiter); ok && key != "" && !strings.HasPrefix(key, "#") {
			props[key] = value
		}
	}
	return props
}

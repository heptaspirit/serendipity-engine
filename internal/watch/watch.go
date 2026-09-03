// Package watch 实现"自动监听"（v0.1.4）：轮询检测变化 → 节流合并 → 触发刷新。
//
// ▍克制设计（防正反馈循环 / 资源耗尽，用户拍板）
//  1. 轮询而非事件监听：固定周期轻量扫描（Obsidian 逐文件 mtime/size 快照、
//     虎鲸 stat 库文件），无 fsnotify 事件风暴，频率完全可控。
//  2. 刷新节流：检测到变化后不立即刷新——进入"待刷新"状态，距上次实际刷新
//     超过节流窗口（默认 60s）才合并执行一次。连续编辑/写入被吸收为
//     每分钟至多一次全量刷新，不会"每次变化都重解析"轰炸 CPU。
//  3. 排除自身产物：Obsidian 扫描跳过 .serendipity/.git/.obsidian/.trash/.dsh
//     等目录——store 写入 .serendipity 不会自触发"变化→刷新→写入→再变化"
//     的无限循环（正反馈的核心来源）。
//  4. 刷新失败只记日志、保留待刷新标记，下一轮重试；不重试轰炸。
//  5. 边权演化不在本包（反馈埋点仅记录，见 internal/store AppendTouch）——
//     "点击→边权变→结果变→再点击"的跑飞由"不演化"在源头切断。
//
// ▍使用
//
//	checker 负责"是否变化"（轻量）；refresh 由调用方注入（serve 的重解析闭包）。
//	Run 阻塞直到 ctx 取消；poll 为检测周期，throttle 为刷新节流窗口。
package watch

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"serendipity-engine/internal/adapter"
)

// excludedDirs 自动监听必须跳过的目录（含 .serendipity 自身存储，防自触发循环）。
var excludedDirs = map[string]bool{
	".serendipity": true, // store 写入会频繁改 mtime → 必须排除
	".git":         true,
	".obsidian":    true,
	".trash":       true,
	".dsh":         true,
	".agents":      true,
}

// Run 启动监听循环：每 poll 检测一次变化；有变化且距上次刷新 >= throttle 时
// 执行 refresh（吸收窗口内所有变化）。阻塞直到 ctx 取消。
// pending（非空）：暴露"有待刷新变化"的标志——serve 注入到 /api/stats 的
// is_pending 字段 + 前端"库有变化"提示条（roadmap 阶段 1 #14）。
//   - 检测到变化 → pending=true（"有变化待刷新"）；
//   - 刷新成功 → pending=false（"已刷新"）。
func Run(ctx context.Context, poll, throttle time.Duration, pending *atomic.Bool, check func() (bool, error), refresh func() error) {
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	var lastRefresh time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			changed, err := check()
			if err != nil {
				// 检测失败（如虎鲸库被占用/暂时不可读）：保留原状态，下轮再试
				log.Printf("[watch] 变化检测失败: %v", err)
				continue
			}
			if changed {
				pending.Store(true)
			}
			if !pending.Load() || time.Since(lastRefresh) < throttle {
				continue // 节流窗口内：合并等待
			}
			// 合并执行一次刷新
			if err := refresh(); err != nil {
				log.Printf("[watch] 自动刷新失败（保留待刷新，节流后重试）: %v", err)
				continue // pending 保留，下轮重试
			}
			pending.Store(false)
			lastRefresh = time.Now()
		}
	}
}

// NewVaultChecker 构造 Obsidian vault 的变化检测器：逐 .md 文件比对
// (mtime, size) 快照，含新增/删除检测。快照存于闭包内（量级 = 文件数）。
// 排除口径与 ParseVault 同源（p 非空）：目录排除 = 内置硬编码（含 .serendipity
// 自身存储，防 store 写入自触发循环）+ 画像 ExcludedDirs；文件级排除走
// VaultProfile.ExcludedName（ExcludedFiles 精确/裸名 + ExcludedPrefixes，.md 免疫）——
// 否则被解析排除的 index.md/log.md 等变化会无效触发刷新（backlog §四 缺口③，
// v0.1.12/13；此前 watch 自带一份更弱的文件名排除副本，已并入画像单一真相源）。
func NewVaultChecker(root string, p *adapter.VaultProfile) func() (bool, error) {
	exclude := map[string]bool{}
	for k := range excludedDirs {
		exclude[k] = true
	}
	for _, e := range p.ExcludedDirs {
		exclude[e] = true
	}
	last := map[string][2]int64{} // path → {mtimeNs, size}
	return func() (bool, error) {
		changed := false
		seen := map[string]bool{}
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != root && exclude[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
				return nil
			}
			if p.ExcludedName(d.Name()) {
				// 被画像排除的文件：不进快照（也不计变化）——与 ParseVault 同源
				delete(last, path)
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			seen[path] = true
			key := [2]int64{info.ModTime().UnixNano(), info.Size()}
			if prev, ok := last[path]; !ok || prev != key {
				changed = true
			}
			last[path] = key
			return nil
		})
		// 删除检测：上次有、这次无
		for p := range last {
			if !seen[p] {
				changed = true
				delete(last, p)
			}
		}
		return changed, err
	}
}

// NewOrcaChecker 构造虎鲸库的变化检测器：stat 库文件 (mtime, size)。
func NewOrcaChecker(dbPath string) func() (bool, error) {
	var lastMod time.Time
	var lastSize int64
	have := false
	return func() (bool, error) {
		info, err := os.Stat(dbPath)
		if err != nil {
			return false, err
		}
		if !have || info.ModTime() != lastMod || info.Size() != lastSize {
			lastMod, lastSize = info.ModTime(), info.Size()
			have = true
			return true, nil
		}
		return false, nil
	}
}

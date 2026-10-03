package scenegraph_test

import (
	"sync"
	"testing"

	sg "github.com/Diudiuseven/scene-graph-go/scenegraph"
)

// 并发读写：写入局部变换、结构编辑与读取世界变换同时进行。
// 在 -race 下运行以检测数据竞争；同时验证不死锁、不 panic。
func TestConcurrentReadWrite(t *testing.T) {
	s := sg.NewScene()
	root := mustCreate(t, s, "root", 0, tr(0, 0, 0))
	const n = 32
	ids := make([]sg.NodeID, n)
	for i := 0; i < n; i++ {
		ids[i] = mustCreate(t, s, "n", root, tr(float64(i), 0, 0))
	}

	stop := make(chan struct{})
	var readers, writers sync.WaitGroup

	// 读者：持续读取世界变换，直到 stop 关闭。
	for g := 0; g < 4; g++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, id := range ids {
					if _, err := s.WorldTransform(id); err != nil {
						t.Errorf("read: %v", err)
						return
					}
				}
			}
		}()
	}
	// 写者：写入局部变换。
	for g := 0; g < 2; g++ {
		writers.Add(1)
		go func(delta float64) {
			defer writers.Done()
			for i := 0; i < 200; i++ {
				for _, id := range ids {
					if err := s.SetLocalTransform(id, tr(delta, float64(i), 0)); err != nil {
						t.Errorf("write: %v", err)
						return
					}
				}
			}
		}(float64(g))
	}
	// 结构编辑者：在 root 与 holder 间搬运。
	writers.Add(1)
	go func() {
		defer writers.Done()
		holder, err := s.CreateNode("holder", 0, tr(0, 0, 0))
		if err != nil {
			t.Errorf("create holder: %v", err)
			return
		}
		for i := 0; i < 50; i++ {
			for _, id := range ids {
				if err := s.Reparent(id, holder); err != nil {
					t.Errorf("reparent out: %v", err)
					return
				}
				if err := s.Reparent(id, root); err != nil {
					t.Errorf("reparent back: %v", err)
					return
				}
			}
		}
	}()

	writers.Wait()
	close(stop)
	readers.Wait()
}

// 并发结束后，世界变换必须与串行重算一致（最终状态自洽）。
func TestConsistentAfterConcurrency(t *testing.T) {
	s := sg.NewScene()
	root := mustCreate(t, s, "root", 0, tr(1, 2, 3))
	a := mustCreate(t, s, "a", root, tr(1, 0, 0))
	b := mustCreate(t, s, "b", a, tr(0, 1, 0))

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = s.SetLocalTransform(b, tr(0, float64(k), 0))
				_, _ = s.WorldTransform(b)
			}
		}(g)
	}
	wg.Wait()

	// 最终：b 的世界 = root * a * b(local)。无论最后谁写入，
	// 缓存必须与当前局部变换一致：重新构造等价场景比对。
	lt, err := s.LocalTransform(b)
	if err != nil {
		t.Fatal(err)
	}
	fresh := sg.NewScene()
	r2 := mustCreate(t, fresh, "root", 0, tr(1, 2, 3))
	a2 := mustCreate(t, fresh, "a", r2, tr(1, 0, 0))
	b2 := mustCreate(t, fresh, "b", a2, lt)

	got := mustWorld(t, s, b)
	want := mustWorld(t, fresh, b2)
	if !got.ApproxEqual(want, eps) {
		t.Fatalf("cached world inconsistent with fresh compute:\ngot  %v\nwant %v", got, want)
	}
}

package handler

import (
	"testing"
)

func TestSessionManagerCreateAndGet(t *testing.T) {
	mgr := NewSessionManager()

	s := mgr.Create()
	if s.ID == "" {
		t.Errorf("session ID 不应为空")
	}

	got, err := mgr.Get(s.ID)
	if err != nil {
		t.Fatalf("获取会话失败: %v", err)
	}
	if got.ID != s.ID {
		t.Errorf("会话 ID 不匹配")
	}
}

func TestSessionManagerRemove(t *testing.T) {
	mgr := NewSessionManager()
	s := mgr.Create()

	mgr.Remove(s.ID)

	_, err := mgr.Get(s.ID)
	if err == nil {
		t.Errorf("已删除的会话应返回错误")
	}
}

func TestSessionManagerGetNotFound(t *testing.T) {
	mgr := NewSessionManager()
	_, err := mgr.Get("nonexistent")
	if err == nil {
		t.Errorf("不存在的会话应返回错误")
	}
}

func TestSessionChannelBuffered(t *testing.T) {
	mgr := NewSessionManager()
	s := mgr.Create()

	// 应能写入多于一缓冲的消息而不阻塞
	for i := 0; i < 5; i++ {
		s.Response <- nil
	}
	// 清理
	for i := 0; i < 5; i++ {
		<-s.Response
	}
}

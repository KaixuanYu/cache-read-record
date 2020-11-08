/*
Copyright 2012 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// peers.go defines how processes find and communicate with their peers.
// peers.go 定义了 进程如何找到peers并且和其交流
package groupcache

import (
	"context"

	pb "github.com/golang/groupcache/groupcachepb"
)

// ProtoGetter is the interface that must be implemented by a peer.
// ProtoGetter 是一个 interface 必须被 peer 实现
type ProtoGetter interface {
	Get(ctx context.Context, in *pb.GetRequest, out *pb.GetResponse) error
}

// PeerPicker is the interface that must be implemented to locate
// the peer that owns a specific key.
// PeerPicker 是一个接口，其实现用来查找拥有指定key的peer
type PeerPicker interface {
	// PickPeer returns the peer that owns the specific key
	// and true to indicate that a remote peer was nominated.
	// It returns nil, false if the key owner is the current peer.
	// PickPeer 找到拥有 指定key的 peer，如果返回true就是找到了，如果返回false，peer就是nil
	PickPeer(key string) (peer ProtoGetter, ok bool)
}

// NoPeers is an implementation of PeerPicker that never finds a peer.
// NoPeers 是PeerPicker的一个实现，不去找一个peer
type NoPeers struct{}

func (NoPeers) PickPeer(key string) (peer ProtoGetter, ok bool) { return }

var (
	portPicker func(groupName string) PeerPicker
)

// RegisterPeerPicker registers the peer initialization function.
// RegisterPeerPicker 注册一个peer 初始化 函数
// It is called once, when the first group is created.
// 当第一个group被创建的时候，它被调用一次
// Either RegisterPeerPicker or RegisterPerGroupPeerPicker should be
// called exactly once, but not both.
// RegisterPeerPicker或RegisterPerGroupPeerPicker中的任何一个都应该精确地调用一次，但不能两者都调用。
func RegisterPeerPicker(fn func() PeerPicker) {
	if portPicker != nil {
		panic("RegisterPeerPicker called more than once")
	}
	portPicker = func(_ string) PeerPicker { return fn() }
}

// RegisterPerGroupPeerPicker registers the peer initialization function,
// RegisterPeerPicker 注册一个peer 初始化 函数
// which takes the groupName, to be used in choosing a PeerPicker.
// 该函数 获取一个 groupName 参数，被用来选择一个 PeerPicker
// It is called once, when the first group is created.
// Either RegisterPeerPicker or RegisterPerGroupPeerPicker should be
// called exactly once, but not both.
func RegisterPerGroupPeerPicker(fn func(groupName string) PeerPicker) {
	if portPicker != nil {
		panic("RegisterPeerPicker called more than once")
	}
	portPicker = fn
}

func getPeers(groupName string) PeerPicker {
	if portPicker == nil {
		return NoPeers{}
	}
	pk := portPicker(groupName)
	if pk == nil {
		pk = NoPeers{}
	}
	return pk
}

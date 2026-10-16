package domain

// DNS 应答校验的测试:TXD 不可预测 + question 段比对。
//
// 这两条是一对:只比 16 位事务 ID 时,攻击者撞对 ID 就能把任意应答塞进校验路径
// (把不属于本机的 IP 说成"指向本机",或用伪造的 NXDOMAIN 让我们以为"没有这条
// 记录");比了 question 段之后,伪造还必须命中正在查的域名与记录类型。

import (
	"testing"
)

// dnsResponse 组装一条应答报文:question 段由 buildDNSQuery 生成(允许与"我们问的"
// 不同,用于模拟伪造/串线的应答),可选用压缩指针追加一条 A 记录。
func dnsResponse(t *testing.T, id uint16, qName string, qType uint16, rcode byte, aRecord *[4]byte) []byte {
	t.Helper()
	q, err := buildDNSQuery(id, qName, qType)
	if err != nil {
		t.Fatalf("buildDNSQuery(%q, %d): %v", qName, qType, err)
	}
	msg := append([]byte{}, q...)
	msg[2] |= 0x80 // QR:这是应答
	msg[3] = rcode
	if aRecord == nil {
		return msg
	}
	msg[6], msg[7] = 0x00, 0x01 // ANCOUNT = 1
	msg = append(msg,
		0xC0, 0x0C, // NAME:压缩指针,指向 question 里的域名
		0x00, byte(dnsTypeA), 0x00, byte(dnsClassIN),
		0x00, 0x00, 0x00, 0x3C, // TTL
		0x00, 0x04, // RDLENGTH
	)
	return append(msg, aRecord[:]...)
}

// 正常应答:question 与查询一致 → 取值成功。
func TestParseDNSAnswerAcceptsMatchingQuestion(t *testing.T) {
	ip := [4]byte{203, 0, 113, 7}
	resp := dnsResponse(t, 0x1234, "shop.customer.com", dnsTypeA, 0, &ip)
	addrs, _, err := parseDNSAnswer(resp, "shop.customer.com", dnsTypeA)
	if err != nil {
		t.Fatalf("parseDNSAnswer: %v", err)
	}
	if len(addrs) != 1 || addrs[0].String() != "203.0.113.7" {
		t.Fatalf("addrs = %v, want [203.0.113.7]", addrs)
	}
}

// 域名大小写不参与判定(DNS 名本身大小写不敏感)。
func TestParseDNSAnswerNameComparisonIsCaseInsensitive(t *testing.T) {
	ip := [4]byte{203, 0, 113, 7}
	resp := dnsResponse(t, 1, "Shop.Customer.COM", dnsTypeA, 0, &ip)
	if _, _, err := parseDNSAnswer(resp, "shop.customer.com", dnsTypeA); err != nil {
		t.Fatalf("大小写不同的同一域名被拒: %v", err)
	}
}

// 伪造应答:ID 撞对但 question 是别的域名 → 丢弃。
func TestParseDNSAnswerRejectsMismatchedQuestionName(t *testing.T) {
	ip := [4]byte{10, 0, 0, 5}
	resp := dnsResponse(t, 0x1234, "evil.example", dnsTypeA, 0, &ip)
	addrs, _, err := parseDNSAnswer(resp, "shop.customer.com", dnsTypeA)
	if err == nil {
		t.Fatalf("question 指向别的域名仍被采信:addrs = %v", addrs)
	}
}

// 伪造应答:域名对但记录类型不符 → 丢弃。
func TestParseDNSAnswerRejectsMismatchedQuestionType(t *testing.T) {
	resp := dnsResponse(t, 1, "shop.customer.com", dnsTypeAAAA, 0, nil)
	if _, _, err := parseDNSAnswer(resp, "shop.customer.com", dnsTypeA); err == nil {
		t.Fatal("question 的记录类型不符仍被采信")
	}
}

// 伪造的 NXDOMAIN 同样要按 question 段校验:否则"没有这条记录"这一结论也能被注入。
func TestParseDNSAnswerRejectsNXDOMAINForOtherName(t *testing.T) {
	resp := dnsResponse(t, 1, "evil.example", dnsTypeA, 3, nil) // rcode=3
	if _, _, err := parseDNSAnswer(resp, "shop.customer.com", dnsTypeA); err == nil {
		t.Fatal("伪造的 NXDOMAIN 被当成'没有这条记录'采信")
	}
}

// 真正的 NXDOMAIN(question 一致)仍是"还没配好",不是错误(既有语义不变)。
func TestParseDNSAnswerAcceptsNXDOMAINForMatchingQuestion(t *testing.T) {
	resp := dnsResponse(t, 1, "shop.customer.com", dnsTypeA, 3, nil)
	addrs, strs, err := parseDNSAnswer(resp, "shop.customer.com", dnsTypeA)
	if err != nil {
		t.Fatalf("NXDOMAIN 应为'无记录'而非错误: %v", err)
	}
	if len(addrs) != 0 || len(strs) != 0 {
		t.Fatalf("NXDOMAIN 不应返回记录: addrs=%v strs=%v", addrs, strs)
	}
}

// question 数不为 1 的报文属于异常(我们的查询只带一个问题)。
func TestParseDNSAnswerRejectsMissingQuestion(t *testing.T) {
	ip := [4]byte{203, 0, 113, 7}
	resp := dnsResponse(t, 1, "shop.customer.com", dnsTypeA, 0, &ip)
	resp[4], resp[5] = 0x00, 0x00 // QDCOUNT = 0
	if _, _, err := parseDNSAnswer(resp, "shop.customer.com", dnsTypeA); err == nil {
		t.Fatal("QDCOUNT=0 的应答被采信")
	}
}

// TXT 路径同样受 question 校验保护(token 是归属证明,更不能被伪造应答左右)。
func TestParseDNSAnswerTXTWithMatchingQuestion(t *testing.T) {
	const name = "_janus-verify.shop.customer.com"
	const token = "tok-abc123"
	rdata := append([]byte{byte(len(token))}, token...)
	q, err := buildDNSQuery(0x4321, name, dnsTypeTXT)
	if err != nil {
		t.Fatalf("buildDNSQuery: %v", err)
	}
	msg := append([]byte{}, q...)
	msg[2] |= 0x80
	msg[6], msg[7] = 0x00, 0x01
	msg = append(msg,
		0xC0, 0x0C,
		0x00, byte(dnsTypeTXT), 0x00, byte(dnsClassIN),
		0x00, 0x00, 0x00, 0x3C,
		byte(len(rdata)>>8), byte(len(rdata)),
	)
	msg = append(msg, rdata...)

	_, strs, err := parseDNSAnswer(msg, name, dnsTypeTXT)
	if err != nil {
		t.Fatalf("parseDNSAnswer: %v", err)
	}
	if len(strs) != 1 || strs[0] != token {
		t.Fatalf("strs = %v, want [%s]", strs, token)
	}
	// 同一个报文配上不同的查询名 → 丢弃
	if _, _, err := parseDNSAnswer(msg, "other.example", dnsTypeTXT); err == nil {
		t.Fatal("TXT 应答的 question 不匹配仍被采信")
	}
}

// 事务 ID 必须是密码学随机,不能由时钟派生。
//
// 时钟派生的两个可观测后果:同一纳秒内两次查询得到同一个 ID(样本里出现大量重复),
// 以及 ID 序列可预测。这里取 64 个样本,要求几乎无重复。
func TestRandomDNSIDIsNotDerivedFromClock(t *testing.T) {
	const n = 64
	seen := map[uint16]int{}
	for i := 0; i < n; i++ {
		id, err := randomDNSID()
		if err != nil {
			t.Fatalf("randomDNSID: %v", err)
		}
		seen[id]++
	}
	if len(seen) < n-4 {
		t.Fatalf("%d 次取样只得到 %d 个不同的事务 ID:ID 可预测,伪造应答只要撞对 16 位即可", n, len(seen))
	}
}

// TC(截断)位在 flags 高字节的 bit 1:置位时必须报"截断",让调用方回退 TCP。
//
// 这一位原先判在了低字节(那里是 RCODE 的 bit 1),后果是两头的:真正的截断
// 应答不被识别,NXDOMAIN/SERVFAIL 这类正常应答反而被当成截断。
func TestParseDNSAnswerDetectsTruncationFlag(t *testing.T) {
	ip := [4]byte{203, 0, 113, 7}
	resp := dnsResponse(t, 1, "shop.customer.com", dnsTypeA, 0, &ip)
	resp[2] |= 0x02 // TC
	_, _, err := parseDNSAnswer(resp, "shop.customer.com", dnsTypeA)
	if err == nil {
		t.Fatal("TC 置位的应答没有被判为截断:调用方不会回退 TCP")
	}
}

// 压缩指针要能跟进(question 段比对依赖它,尽管真实 question 段通常不压缩)。
func TestDecodeDNSNameFollowsCompressionPointer(t *testing.T) {
	tail, err := encodeDNSName("example.com")
	if err != nil {
		t.Fatalf("encodeDNSName: %v", err)
	}
	// 偏移 0:标签 "www";偏移 4:指针 → 偏移 6(尾部域名)
	msg := []byte{3, 'w', 'w', 'w', 0xC0, 0x06}
	msg = append(msg, tail...)
	name, next, err := decodeDNSName(msg, 0)
	if err != nil {
		t.Fatalf("decodeDNSName: %v", err)
	}
	if name != "www.example.com" {
		t.Fatalf("name = %q, want www.example.com", name)
	}
	if next != 6 {
		t.Fatalf("next = %d, want 6(指针之后的位置)", next)
	}
}

package domain

// DNS 查询层:一次显式构造的、带 hosts 免疫的解析。
//
// 为什么不用 net.DefaultResolver / net.Resolver{PreferGo:true}:
// Go 的 hostLookupOrder 在"优先 Go 解析器"时是 files+DNS(files 在前),也就是说
// PreferGo 仍然先读 /etc/hosts(实测:PreferGo:true 解析 shop.example.com 得到的
// 仍是 hosts 里的 127.0.0.1,而不是公网 A 记录)。/etc/hosts 是部署者手里的一张
// 本地表,加一行就能让任意域名"解析到本机";用它参与判定等于把判定权交给本地文件。
//
// 所以这里的分工是:
//   - TXT(归属证明,权威判定)→ 标准库 LookupTXT。它走 goLookupTXT,不查 hosts,
//     用标准库最稳(编码/压缩指针/TC 回退都交给它)。
//   - A/AAAA(前置快速失败 + 兜底)→ 手写一次最小查询,直接打 /etc/resolv.conf
//     里的 nameserver,hosts 完全不参与。
//
// 为什么 A/AAAA 不直接删掉:对"已经配好"的租户它是最省事的一步(不改 DNS 就能
// 拿到"你的 A 记录已指向本服务器,再补一条 TXT 就能激活"这类精确指引),而且
// 归属已经被 TXT 证明之后,它仍是有意义的兜底(证明归属但没指向本机的域名,
// 平台确实服务不了)。

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"
)

// DNS 记录类型(只用到本模块需要的几个)。
const (
	dnsTypeA    uint16 = 1
	dnsTypeTXT  uint16 = 16
	dnsTypeAAAA uint16 = 28
	dnsClassIN  uint16 = 1
)

const (
	// dnsUDPPayload UDP 单次读取上限。不用 EDNS0,响应会被服务器截到 512 字节,
	// 所以留足余量,并在看到 TC 位时回退 TCP。
	dnsUDPPayload = 4096
	// dnsMaxNameservers 单轮最多尝试的 nameserver 数量:第一个不响应就换下一个,
	// 免得一个坏服务器把每次校验拖到超时。
	dnsMaxNameservers = 3
)

var (
	errDNSTruncated = errors.New("dns: 报文截断")
	errDNSMalformed = errors.New("dns: 报文格式非法")
)

// nameserversFromResolvConf 解析 /etc/resolv.conf 的 nameserver 行。
// 文件不可读或没有条目时返回 nil:没有解析器时让校验失败,而不是偷偷回落到
// 某个公共 DNS(那等于把租户提交的域名发到平台外部)。
func nameserversFromResolvConf() []string {
	b, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) < 2 || f[0] != "nameserver" {
			continue
		}
		if _, err := netip.ParseAddr(f[1]); err != nil {
			continue
		}
		out = append(out, f[1])
		if len(out) >= dnsMaxNameservers {
			break
		}
	}
	return out
}

// encodeDNSName 把域名编码成 DNS wire 格式的点分标签序列。
func encodeDNSName(name string) ([]byte, error) {
	name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
	if name == "" {
		return nil, fmt.Errorf("dns: 空域名")
	}
	if _, err := netip.ParseAddr(name); err == nil {
		return nil, fmt.Errorf("dns: %q 是 IP 字面量,不是域名", name)
	}
	out := []byte{}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 {
			return nil, fmt.Errorf("dns: 标签非法 %q", label)
		}
		out = append(out, byte(len(label)))
		out = append(out, label...)
	}
	return append(out, 0), nil
}

// dnsMaxNameJumps 解析压缩指针时最多跟进的跳数(防环)。
const dnsMaxNameJumps = 16

// randomDNSID 生成一个不可预测的事务 ID。
//
// 原先是 `uint16(time.Now().UnixNano())`:低 16 位不仅可预测(纳秒时钟可以从
// 响应时间上夹逼出来,且相邻查询的差值固定),而且同一纳秒内两次查询会得到同一个
// ID —— 攻击者只要撞对 16 位就能把自己的伪造应答塞进来,而当时的应答校验只比
// TXID、不看 question 段,撞对即中。改成 crypto/rand:开销可以忽略(每次查询
// 2 字节),且与下面 parseDNSAnswer 的 question 比对一起构成两道校验。
func randomDNSID() (uint16, error) {
	var b [2]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return 0, fmt.Errorf("dns: 生成事务 ID 失败: %w", err)
	}
	return uint16(b[0])<<8 | uint16(b[1]), nil
}

// buildDNSQuery 构造一次 A/AAAA/TXT 查询报文。
func buildDNSQuery(id uint16, name string, qtype uint16) ([]byte, error) {
	qname, err := encodeDNSName(name)
	if err != nil {
		return nil, err
	}
	msg := make([]byte, 0, 12+len(qname)+4)
	msg = append(msg,
		byte(id>>8), byte(id), // ID
		0x01, 0x00, // flags: RD(递归 Desired)
		0x00, 0x01, // QDCOUNT
		0x00, 0x00, // ANCOUNT
		0x00, 0x00, // NSCOUNT
		0x00, 0x00, // ARCOUNT
	)
	msg = append(msg, qname...)
	msg = append(msg, byte(qtype>>8), byte(qtype), byte(dnsClassIN>>8), byte(dnsClassIN))
	return msg, nil
}

// skipDNSName 跳过报文里的一个域名(压缩指针指向别处,不再跟进)。
func skipDNSName(msg []byte, off int) (int, error) {
	for {
		if off >= len(msg) {
			return 0, errDNSTruncated
		}
		l := int(msg[off])
		switch {
		case l == 0:
			return off + 1, nil
		case l&0xC0 == 0xC0:
			return off + 2, nil
		case l&0xC0 != 0:
			return 0, errDNSMalformed
		default:
			off += 1 + l
		}
	}
}

// decodeDNSName 读出一个域名并做小写归一,返回下一个字段的位置。
//
// 与 skipDNSName 的区别:它需要真的名字(用于比对 question 段),所以要跟进压缩
// 指针 —— 指针可以嵌套,跳数用 dnsMaxNameJumps 封顶防环。
func decodeDNSName(msg []byte, off int) (string, int, error) {
	var (
		labels []string
		next   = -1 // 首次遇到指针的位置之后 = 本条记录的结束位置
		jumps  int
	)
	for {
		if off >= len(msg) {
			return "", 0, errDNSTruncated
		}
		l := int(msg[off])
		switch {
		case l == 0:
			if next < 0 {
				next = off + 1
			}
			return strings.ToLower(strings.Join(labels, ".")), next, nil
		case l&0xC0 == 0xC0:
			if off+2 > len(msg) {
				return "", 0, errDNSTruncated
			}
			if next < 0 {
				next = off + 2
			}
			jumps++
			if jumps > dnsMaxNameJumps {
				return "", 0, fmt.Errorf("%w: 域名压缩指针跳数过多", errDNSMalformed)
			}
			off = int(msg[off]&0x3F)<<8 | int(msg[off+1])
		case l&0xC0 != 0:
			return "", 0, errDNSMalformed
		default:
			if off+1+l > len(msg) {
				return "", 0, errDNSTruncated
			}
			labels = append(labels, strings.ToLower(string(msg[off+1:off+1+l])))
			off += 1 + l
		}
	}
}

// parseDNSAnswer 从应答报文里取出 wanted 类型的记录值,并先校验它回答的确实是我们
// 问的那个问题。
//
// question 段比对为什么必须做(不能只比 TXID):一次查询只产生一个 16 位随机 ID,
// 而 UDP 应答可以被抢跑注入 —— 攻击者不必构造完美的应答,只要 ID 撞对就能把任意
// 结果塞进校验路径(比如把一个不属于本机的 IP 说成"指向本机")。比 NAME/QTYPE
// 之后,伪造还必须命中我们正在查的域名与记录类型,门槛与真实解析器齐平。
//
// A/AAAA 返回地址;TXT 返回字符串(逐段 + 拼接值,便于匹配被拆成多段的 token)。
func parseDNSAnswer(msg []byte, wantName string, wanted uint16) ([]netip.Addr, []string, error) {
	if len(msg) < 12 {
		return nil, nil, errDNSTruncated
	}
	if msg[2]&0x02 != 0 { // TC
		// TC 在 flags 高字节的 bit 1(与 QR/Opcode/AA/RD 同字节);原先写成
		// msg[3]&0x02,那里其实是 RCODE 的 bit 1 —— 于是真正的截断应答不会被
		// 识别(该走 TCP 回退却不走),而 NXDOMAIN(rcode=3)/SERVFAIL(rcode=2)
		// 这类正常应答反而被误判成"截断"。question 段校验要排在 rcode 判定之前,
		// 这个位判错会让两条路径都失去意义,所以随本次应答校验一起改正。
		return nil, nil, errDNSTruncated
	}
	qdCount := int(msg[4])<<8 | int(msg[5])
	anCount := int(msg[6])<<8 | int(msg[7])
	off := 12
	// question 段校验必须排在 rcode 判定之前:否则一条伪造的 NXDOMAIN(rcode=3)
	// 只要撞对 TXID 就会被当成"没有这条记录"直接采信 —— 同样是可注入的结果。
	// 我们的查询只带一个问题,应答里的 question 是它的回显,数量不符即异常报文。
	if qdCount != 1 {
		return nil, nil, fmt.Errorf("dns: 应答 question 数 %d,与查询(1)不符", qdCount)
	}
	gotName, next, err := decodeDNSName(msg, off)
	if err != nil {
		return nil, nil, err
	}
	if next+4 > len(msg) {
		return nil, nil, errDNSTruncated
	}
	gotType := uint16(msg[next])<<8 | uint16(msg[next+1])
	off = next + 4
	want := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(wantName), "."))
	if wantName != "" && (gotName != want || gotType != wanted) {
		return nil, nil, fmt.Errorf("dns: 应答 question(%s/%d)与查询(%s/%d)不匹配,丢弃",
			gotName, gotType, want, wanted)
	}
	switch rcode := msg[3] & 0x0F; rcode {
	case 0: // NOERROR
	case 3: // NXDOMAIN:域名不存在,属于"还没配好",不是错误
		return nil, nil, nil
	default:
		return nil, nil, fmt.Errorf("dns: 应答 rcode=%d", rcode)
	}
	var (
		addrs []netip.Addr
		strs  []string
	)
	for i := 0; i < anCount; i++ {
		var err error
		if off, err = skipDNSName(msg, off); err != nil {
			return nil, nil, err
		}
		if off+10 > len(msg) {
			return nil, nil, errDNSTruncated
		}
		rtype := uint16(msg[off])<<8 | uint16(msg[off+1])
		class := uint16(msg[off+2])<<8 | uint16(msg[off+3])
		rdlen := int(uint16(msg[off+8])<<8 | uint16(msg[off+9]))
		off += 10
		if off+rdlen > len(msg) {
			return nil, nil, errDNSTruncated
		}
		rdata := msg[off : off+rdlen]
		off += rdlen
		if rtype != wanted || class != dnsClassIN {
			continue
		}
		switch wanted {
		case dnsTypeA:
			if rdlen == 4 {
				addrs = append(addrs, netip.AddrFrom4([4]byte(rdata)))
			}
		case dnsTypeAAAA:
			if rdlen == 16 {
				addrs = append(addrs, netip.AddrFrom16([16]byte(rdata)))
			}
		case dnsTypeTXT:
			strs = append(strs, decodeTXTRecord(rdata)...)
		}
	}
	return addrs, strs, nil
}

// decodeTXTRecord 拆解 TXT rdata:每个 character-string 一段,多于一段时再补一个拼接值。
func decodeTXTRecord(rdata []byte) []string {
	var parts []string
	var joined strings.Builder
	for i := 0; i < len(rdata); {
		l := int(rdata[i])
		i++
		if i+l > len(rdata) {
			break
		}
		parts = append(parts, string(rdata[i:i+l]))
		joined.Write(rdata[i : i+l])
		i += l
	}
	if len(parts) > 1 {
		parts = append(parts, joined.String())
	}
	return parts
}

// exchangeUDP 打一次 UDP 查询。TC 置位时由调用方回退 TCP。
func exchangeUDP(ctx context.Context, server string, query []byte, timeout time.Duration) ([]byte, error) {
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(deadlineOf(ctx, timeout))
	if _, err := conn.Write(query); err != nil {
		return nil, err
	}
	buf := make([]byte, dnsUDPPayload)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	if n >= 4 && uint16(buf[0])<<8|uint16(buf[1]) != uint16(query[0])<<8|uint16(query[1]) {
		return nil, errors.New("dns: 事务 ID 不匹配")
	}
	return buf[:n], nil
}

// exchangeTCP 在 UDP 被截断时用 TCP 重试(2 字节长度前缀)。
func exchangeTCP(ctx context.Context, server string, query []byte, timeout time.Duration) ([]byte, error) {
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", net.JoinHostPort(server, "53"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(deadlineOf(ctx, timeout))
	framed := make([]byte, 0, len(query)+2)
	framed = append(framed, byte(len(query)>>8), byte(len(query)))
	framed = append(framed, query...)
	if _, err := conn.Write(framed); err != nil {
		return nil, err
	}
	var lenBuf [2]byte
	if _, err := readFull(conn, lenBuf[:]); err != nil {
		return nil, err
	}
	n := int(lenBuf[0])<<8 | int(lenBuf[1])
	if n == 0 || n > dnsUDPPayload {
		return nil, errDNSMalformed
	}
	resp := make([]byte, n)
	if _, err := readFull(conn, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func deadlineOf(ctx context.Context, timeout time.Duration) time.Time {
	if dl, ok := ctx.Deadline(); ok {
		return dl
	}
	return time.Now().Add(timeout)
}

func readFull(conn net.Conn, b []byte) (int, error) {
	total := 0
	for total < len(b) {
		n, err := conn.Read(b[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// queryRecord 向 servers 逐个发一次查询,取第一个能应答的服务器的结果。
func queryRecord(ctx context.Context, servers []string, name string, qtype uint16, timeout time.Duration) ([]netip.Addr, []string, error) {
	if len(servers) == 0 {
		return nil, nil, errors.New("dns: 没有可用的 nameserver")
	}
	randomID, err := randomDNSID()
	if err != nil {
		return nil, nil, err
	}
	query, err := buildDNSQuery(randomID, name, qtype)
	if err != nil {
		return nil, nil, err
	}
	var lastErr error
	for _, srv := range servers {
		resp, err := exchangeUDP(ctx, srv, query, timeout)
		if errors.Is(err, errDNSTruncated) {
			resp, err = exchangeTCP(ctx, srv, query, timeout)
		}
		if err != nil {
			lastErr = err
			continue
		}
		return parseDNSAnswer(resp, name, qtype)
	}
	return nil, nil, lastErr
}

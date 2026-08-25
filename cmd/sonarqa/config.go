package main

import (
	"flag"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const defaultAddress = "127.0.0.1:19081"

type config struct {
	Address       string
	Selfcheck     bool
	DataDirectory string
	DataExplicit  bool
}

func parseConfig(args []string, portEnvironment string) (config, error) {
	flags := flag.NewFlagSet("sonarqa", flag.ContinueOnError)
	flags.SetOutput(new(strings.Builder))
	address := flags.String("addr", "", "HTTP 回环监听地址")
	selfcheck := flags.Bool("selfcheck", false, "执行真实 HTTP 全流程自检后退出")
	data := flags.String("data", "./data", "事件日志和投影快照目录")
	if err := flags.Parse(args); err != nil {
		return config{}, fmt.Errorf("解析启动参数失败: %w", err)
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("不支持位置参数: %s", strings.Join(flags.Args(), " "))
	}
	addressExplicit, dataExplicit := false, false
	flags.Visit(func(value *flag.Flag) {
		if value.Name == "addr" {
			addressExplicit = true
		}
		if value.Name == "data" {
			dataExplicit = true
		}
	})
	resolved, err := resolveAddress(*address, addressExplicit, portEnvironment)
	if err != nil {
		return config{}, err
	}
	if strings.TrimSpace(*data) == "" {
		return config{}, fmt.Errorf("存储目录不能为空")
	}
	return config{Address: resolved, Selfcheck: *selfcheck, DataDirectory: *data, DataExplicit: dataExplicit}, nil
}

func resolveAddress(flagAddress string, explicit bool, portEnvironment string) (string, error) {
	address := ""
	if explicit {
		if strings.TrimSpace(flagAddress) == "" {
			return "", fmt.Errorf("-addr 不能为空")
		}
		address = strings.TrimSpace(flagAddress)
	} else if strings.TrimSpace(portEnvironment) != "" {
		port, err := parsePort(strings.TrimSpace(portEnvironment))
		if err != nil {
			return "", fmt.Errorf("PORT 无效: %w", err)
		}
		address = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	} else {
		address = defaultAddress
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("监听地址必须采用 host:port 格式: %w", err)
	}
	if host == "" {
		return "", fmt.Errorf("监听地址必须明确指定回环主机")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("监听地址必须使用 127.0.0.1 或其他回环 IP，禁止绑定外网地址")
	}
	port, err := parsePort(portText)
	if err != nil {
		return "", fmt.Errorf("监听端口无效: %w", err)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("端口必须是 1 到 65535 的整数")
	}
	return port, nil
}

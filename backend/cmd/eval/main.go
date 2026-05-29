// Command eval 离线评测知识库检索质量。
// 它复用线上同一套 embedder + Milvus 检索链路，读取人工标注的数据集，
// 在多个 TopK 档位上输出 HitRate / Recall@K / MRR，方便量化迭代效果。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"bagu-agent/backend/internal/config"
	"bagu-agent/backend/internal/embedder"
	"bagu-agent/backend/internal/eval"
	"bagu-agent/backend/internal/retriever"
	"bagu-agent/backend/internal/vectorstore"
)

func main() {
	var (
		configPath  string
		datasetPath string
		outPath     string
		userID      uint64
		ksRaw       string
	)
	flag.StringVar(&configPath, "config", "", "配置文件路径（默认 configs/config.yaml）")
	flag.StringVar(&datasetPath, "dataset", "eval/dataset.json", "评测数据集 JSON 路径")
	flag.StringVar(&outPath, "out", "", "可选：把 Markdown 报告写到该文件")
	flag.Uint64Var(&userID, "user", 1, "检索哪个用户的知识库")
	flag.StringVar(&ksRaw, "k", "1,3,5,10", "逗号分隔的 TopK 档位")
	flag.Parse()

	if err := run(configPath, datasetPath, outPath, userID, ksRaw); err != nil {
		fmt.Fprintln(os.Stderr, "eval failed:", err)
		os.Exit(1)
	}
}

func run(configPath, datasetPath, outPath string, userID uint64, ksRaw string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cases, err := loadDataset(datasetPath)
	if err != nil {
		return err
	}

	ks, err := parseKs(ksRaw)
	if err != nil {
		return err
	}

	embeddingClient, err := embedder.New(cfg.AI, cfg.Milvus.EmbeddingDim)
	if err != nil {
		return fmt.Errorf("init embedder: %w", err)
	}
	milvusStore := vectorstore.NewLazyMilvusStore(cfg.Milvus)
	retrieverService := retriever.NewService(embeddingClient, milvusStore)

	report, err := eval.Benchmark(context.Background(), retrieverService, userID, cases, ks)
	if err != nil {
		return err
	}

	md := report.Markdown()
	fmt.Println(md)
	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(md+"\n"), 0o644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		fmt.Fprintln(os.Stderr, "report written to", outPath)
	}
	return nil
}

func loadDataset(path string) ([]eval.BenchmarkCase, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read dataset %s: %w", path, err)
	}
	var cases []eval.BenchmarkCase
	if err := json.Unmarshal(b, &cases); err != nil {
		return nil, fmt.Errorf("parse dataset %s: %w", path, err)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("dataset %s is empty", path)
	}
	return cases, nil
}

func parseKs(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	ks := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		k, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid -k value %q: %w", p, err)
		}
		ks = append(ks, k)
	}
	if len(ks) == 0 {
		return nil, fmt.Errorf("no valid -k values")
	}
	return ks, nil
}

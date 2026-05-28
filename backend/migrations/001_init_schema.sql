CREATE TABLE IF NOT EXISTS users (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  email VARCHAR(128) NOT NULL UNIQUE,
  nickname VARCHAR(64) NOT NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS documents (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(255) NOT NULL,
  source_type VARCHAR(32) NOT NULL,
  source_path VARCHAR(512) NOT NULL,
  category VARCHAR(64) NULL,
  status VARCHAR(32) NOT NULL,
  chunk_count BIGINT NOT NULL DEFAULT 0,
  error_message TEXT NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  INDEX idx_documents_user_id (user_id),
  INDEX idx_documents_category (category),
  INDEX idx_documents_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS document_chunks (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  document_id BIGINT UNSIGNED NOT NULL,
  chunk_uid VARCHAR(64) NOT NULL UNIQUE,
  title_path VARCHAR(1024) NOT NULL,
  heading_level BIGINT NOT NULL,
  content LONGTEXT NOT NULL,
  content_with_title LONGTEXT NOT NULL,
  chunk_index BIGINT NOT NULL,
  token_count BIGINT NOT NULL,
  milvus_collection VARCHAR(128) NULL,
  milvus_pk VARCHAR(128) NULL,
  created_at DATETIME(3) NULL,
  INDEX idx_document_chunks_document_id (document_id),
  INDEX idx_document_chunks_title_path (title_path(255)),
  INDEX idx_document_chunks_milvus_pk (milvus_pk)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS conversations (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  title VARCHAR(255) NOT NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  INDEX idx_conversations_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS messages (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  conversation_id BIGINT UNSIGNED NOT NULL,
  role VARCHAR(32) NOT NULL,
  content LONGTEXT NOT NULL,
  citations_json JSON NULL,
  created_at DATETIME(3) NULL,
  INDEX idx_messages_conversation_id (conversation_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS agent_runs (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  conversation_id BIGINT UNSIGNED NOT NULL,
  message_id BIGINT UNSIGNED NOT NULL,
  user_query TEXT NOT NULL,
  intent VARCHAR(64) NOT NULL,
  tools_used JSON NULL,
  retrieved_chunks_json JSON NULL,
  final_answer LONGTEXT NULL,
  latency_ms BIGINT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NULL,
  INDEX idx_agent_runs_conversation_id (conversation_id),
  INDEX idx_agent_runs_message_id (message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS rag_eval_cases (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  question TEXT NOT NULL,
  expected_chunk_uids JSON NOT NULL,
  category VARCHAR(64) NULL,
  difficulty VARCHAR(32) NULL,
  created_at DATETIME(3) NULL,
  INDEX idx_rag_eval_cases_category (category)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS rag_eval_results (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  eval_case_id BIGINT UNSIGNED NOT NULL,
  top_k BIGINT NOT NULL,
  retrieved_chunk_uids JSON NOT NULL,
  hit BOOLEAN NOT NULL,
  recall_at_k DOUBLE NOT NULL,
  mrr DOUBLE NOT NULL,
  answer LONGTEXT NULL,
  faithfulness_score DOUBLE NULL,
  relevance_score DOUBLE NULL,
  citation_score DOUBLE NULL,
  created_at DATETIME(3) NULL,
  INDEX idx_rag_eval_results_eval_case_id (eval_case_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


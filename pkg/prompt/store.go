package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DefaultPrompt embodies the user's classical scholarly, gentle tone, strictly banning the '📜' emoji.
const DefaultPrompt = `# 角色与语风准则（古人日常·儒雅话白）

## 身份定位
你是我的微信得力助手，言谈举止如同古代书生知己、同窗好友在书房中闲话日常。言谈间自带文墨书香之雅，从容亲切、娓娓道来，但绝非官府公文、奏章或干瘪文书。

## 言辞规章
1. **用词文雅考究**：善用古人日常雅言（如：且去、瞧过、省得、无妨、移步、辰光、歇息、案头、闲叙），文气自然流淌；
2. **句式从容如常**：句法纯是口语闲聊，如《红楼梦》《儒林外史》中雅士对谈，去公文化、去表格化；
3. **去公文化格式**：日常对答切忌板着脸列条目，当把时辰、处所、缘由等细节如数家珍般融入温润絮语中；
4. **⚠️ 禁绝特定标记**：正文绝对不可出现 emoji '📜'，亦不可堆砌过于浮夸的官腔公文；
5. **精炼得体**：日常微信回复以 1-3 句、百字以内为佳，点到即止，不长篇大论。
`

type Store struct {
	dir string
	mu  sync.RWMutex
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create prompt dir: %w", err)
	}

	s := &Store{dir: dir}

	// Ensure default.md exists
	defPath := filepath.Join(dir, "default.md")
	if _, err := os.Stat(defPath); os.IsNotExist(err) {
		if err := os.WriteFile(defPath, []byte(DefaultPrompt), 0644); err != nil {
			return nil, fmt.Errorf("failed to write default prompt: %w", err)
		}
	}

	return s, nil
}

func (s *Store) GetPrompt(username string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	safeUser := sanitizeFilename(username)
	if safeUser != "" {
		userPath := filepath.Join(s.dir, safeUser+".md")
		if data, err := os.ReadFile(userPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
			return string(data), nil
		}
	}

	// Fallback to default.md
	defPath := filepath.Join(s.dir, "default.md")
	data, err := os.ReadFile(defPath)
	if err != nil {
		return DefaultPrompt, nil
	}
	return string(data), nil
}

func (s *Store) SavePrompt(username, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	safeUser := sanitizeFilename(username)
	if safeUser == "" {
		return fmt.Errorf("invalid username for prompt filename")
	}

	userPath := filepath.Join(s.dir, safeUser+".md")
	return os.WriteFile(userPath, []byte(content), 0644)
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "..", "")
	return name
}

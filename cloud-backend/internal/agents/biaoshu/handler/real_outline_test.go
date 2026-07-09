package handler

import (
"os"
"regexp"
"testing"
)

func TestRealOutline(t *testing.T) {
re := regexp.MustCompile( + "^##\\s*[一二三四五六七八九十]+[章、\\s.．·]" + @)
data, _ := os.ReadFile( + "E:\\lingxi\\tangying-ai-operation-system\\biaoshu-tools\\output\\养护\\03_技术标四级大纲.md" + @)
text := string(data)
lines := []string{}
cur := ""
for _, b := range text {
if b == '\n' {
lines = append(lines, cur)
cur = ""
} else {
cur += string(b)
}
}
if cur != "" {
lines = append(lines, cur)
}
match := 0
for _, l := range lines {
if re.MatchString(l) {
match++
t.Logf("MATCH: %s", l[:min(60, len(l))])
}
}
if match == 0 {
t.Error("NO MATCHES")
for _, l := range lines {
if len(l) > 2 && l[0] == '#' && l[1] == '#' {
t.Logf("LINE: %s", l[:min(60, len(l))])
}
}
}
}

package main

import (
"fmt"
"regexp"
)

func main() {
re := regexp.MustCompile("^##\\s*[一二三四五六七八九十]+[章、\\s.．·]")
testLines := []string{
"## 第一章 设计方案：全年草花布置工作计划及方案",
"## 第二章 日常种植方案",
"## 一、总体项目管理方案",
"### 1.1 子节",
"## 概述",
}
for _, l := range testLines {
fmt.Printf("%-10v line=%q\n", re.MatchString(l), l)
}
}
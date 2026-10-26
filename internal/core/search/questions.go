package search

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// QuestionPlan separates names and applicability constraints from search terms.
type QuestionPlan struct {
	Bindings     []RequirementBinding   `json:"bindings"`
	Requirements []AttributeRequirement `json:"requirements"`
	Subjects     []string               `json:"subjects"`
	Attribute    string                 `json:"attribute"`
	Stage        *string                `json:"stage"`
	Comparison   bool                   `json:"comparison"`
	Scope        string                 `json:"scope"`
	Constraints  []string               `json:"constraints"`
	Filters      []string               `json:"filters"`
}
type questionSpec struct {
	QuestionPlan
	focus              string
	natural            bool
	expectedCount      int
	boundClauses       bool
	identityUnverified map[string]bool
}
type attributeVocabulary struct {
	name    string
	aliases []string
}

// Vocabulary describes attributes and language, never page names or answers.
// Internal vocabulary is immutable after initialization; consumers only read it.
var compiledAttributeVocabulary = buildAttributeWords()
var compiledAttributeAliases = flattenAttributeAliases()
var caseInvariantAttributes = buildCaseInvariantAttributes()

// Exact search is equivalent to EqualFold only when every rune is unchanged by
// Unicode case folding. Determine that once for the static attribute vocabulary;
// mixed-case and arbitrary caller-supplied words keep the general matcher.
func buildCaseInvariantAttributes() map[string]bool {
	words := make(map[string]bool)
	for _, word := range compiledAttributeAliases {
		invariant := word != ""
		for _, r := range word {
			invariant = invariant && unicode.SimpleFold(r) == r
		}
		if invariant {
			words[word] = true
		}
	}
	return words
}

func flattenAttributeAliases() []string {
	var aliases []string
	for _, entry := range attributeWords() {
		aliases = append(aliases, entry.aliases...)
	}
	return aliases
}

func attributeWords() []attributeVocabulary { return compiledAttributeVocabulary }

func buildAttributeWords() []attributeVocabulary {
	return []attributeVocabulary{
		{"远程攻击", []string{"远程攻击", "远程", "远处", "打远"}},
		{"伤害", []string{"伤害"}},
		{"空中单位", append(phraseAliases([]string{"空中", "飞行"}, []string{"单位", "目标", "敌人", "怪物", "怪"}), "天上飞", "空中", "对空")},
		{"初始部署费用", costAliases()},
		{"再部署时间", redeploymentAliases()},
		{"攻击间隔", []string{"攻击间隔", "攻击速度", "攻速", "出手", "平A", "普攻间隔"}},
		{"生命上限", append(append(phraseAliases([]string{"生命", "血量", "血条", "HP"}, []string{"上限", "封顶", "最大值", "值", "点数", "总量", "总值"}), "血量", "HP"), fullHealthAliases()...)},
		{"阻挡数", append(phraseAliases([]string{"挡", "拦"}, []string{"几个", "多少个", "多少"}), "阻挡数量", "阻挡数值", "阻挡数", "阻挡", "挡住", "拦住", "多挡")},
		{"所属势力", []string{"所属势力", "属于", "势力", "成员", "公司", "阵营", "隶属"}},
		{"获得方式", append(phraseAliases([]string{"获得", "获取", "招募"}, []string{"方式", "渠道", "途径"}), "公开招募", "中坚寻访", "教学关卡", "关卡", "获得", "获取", "入手", "拿到", "招到")},
		{"特性", []string{"特性"}},
		{"攻击", []string{"攻击力", "攻击"}},
	}
}

func costAliases() []string {
	heads := []string{"部署", "出场", "上场", "落地", "开局", "初始", "起手", "开场"}
	heads = append(heads, phraseAliases([]string{"首次", "初次", "第一次", "初"}, []string{"部署", "登场", "出场", "上场", "落地"})...)
	tails := []string{"费用", "成本", "开销", "花销", "消耗", "价格", "价", "费"}
	return append(phraseAliases(heads, append(slices.Clone(tails), phraseAliases([]string{"的"}, tails)...)), "初始部署费用", "部署费", "几费", "花费")
}

var compiledFullHealthAliases = phraseAliases([]string{"满"}, []string{"血", "生命"})

func fullHealthAliases() []string { return compiledFullHealthAliases }

func redeploymentAliases() []string {
	actions := redeploymentActions()
	tails := []string{"时间", "耗时", "用时"}
	return append(phraseAliases(actions, append(slices.Clone(tails), phraseAliases([]string{"的"}, tails)...)), actions...)
}

var compiledRedeploymentActions = buildRedeploymentActions()

func redeploymentActions() []string { return compiledRedeploymentActions }

func buildRedeploymentActions() []string {
	return append(phraseAliases([]string{"再", "重新", "再次"}, []string{"部署", "登场", "上场", "落地"}), "返场")
}

// Compose attribute phrases from semantic word families rather than questions.
func phraseAliases(heads, tails []string) []string {
	aliases := make([]string, 0, len(heads)*len(tails))
	for _, head := range heads {
		for _, tail := range tails {
			aliases = append(aliases, head+tail)
		}
	}
	return aliases
}

var stageExpression = regexp.MustCompile(`(?:精英|精)([零〇一二三四五六七八九十0-9]+)`)
var countExpression = regexp.MustCompile(`([零〇一二三四五六七八九十0-9]+)个(样本(?:干员)?)`)
var numberExpression = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?(?:s|秒|%)?`)
var membershipExpression = regexp.MustCompile(`(?:属于|隶属)([^？?，,]+)|是([^？?，,]+)成员`)

func smallNumber(value string) (int, bool) {
	if n, err := strconv.Atoi(value); err == nil {
		return n, true
	}
	digits := map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	n, tens := 0, 0
	for _, r := range value {
		if r == '十' {
			if n == 0 {
				n = 1
			}
			tens = n * 10
			n = 0
			continue
		}
		v, ok := digits[r]
		if !ok {
			return 0, false
		}
		n = v
	}
	return tens + n, true
}

func asciiIdentifier(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_'
}

func languageAt(value string, at int, word string) bool {
	if word == "" || at+len(word) > len(value) {
		return false
	}
	candidate := value[at : at+len(word)]
	if caseInvariantAttributes[word] {
		if candidate != word {
			return false
		}
	} else if !strings.EqualFold(candidate, word) {
		return false
	}
	ascii := true
	for _, r := range word {
		if r > 127 {
			ascii = false
			break
		}
	}
	if !ascii {
		return (!asciiIdentifier(word[0]) || at == 0 || !asciiIdentifier(value[at-1])) &&
			(!asciiIdentifier(word[len(word)-1]) || at+len(word) == len(value) || !asciiIdentifier(value[at+len(word)]))
	}
	return (at == 0 || !asciiIdentifier(value[at-1])) && (at+len(word) == len(value) || !asciiIdentifier(value[at+len(word)]))
}

func containsLanguage(value, word string) bool {
	if caseInvariantAttributes[word] {
		for from := 0; from < len(value); {
			relative := strings.Index(value[from:], word)
			if relative < 0 {
				return false
			}
			at := from + relative
			if languageAt(value, at, word) {
				return true
			}
			_, size := utf8.DecodeRuneInString(value[at:])
			from = at + size
		}
		return false
	}
	for at := range value {
		if languageAt(value, at, word) {
			return true
		}
	}
	return false
}

// Maximize whole-token coverage, preferring longer tokens on ties. This handles
// overlaps (a verb followed by a compound noun) without creating fragments that
// are then reinterpreted by another replacement pass.
func stripLanguage(value string, words []string) string {
	words = slices.Clone(words)
	slices.SortStableFunc(words, func(a, b string) int { return len(b) - len(a) })
	score := make([]int, len(value)+1)
	step := make([]int, len(value))
	matched := make([]bool, len(value))
	for at := len(value) - 1; at >= 0; at-- {
		if !utf8.RuneStart(value[at]) {
			continue
		}
		_, size := utf8.DecodeRuneInString(value[at:])
		step[at] = size
		score[at] = score[at+size]
		for _, word := range words {
			if !languageAt(value, at, word) {
				continue
			}
			candidate := len([]rune(word)) + score[at+len(word)]
			if candidate > score[at] || candidate == score[at] && len(word) > step[at] {
				score[at] = candidate
				step[at] = len(word)
				matched[at] = true
			}
		}
	}
	var out strings.Builder
	for at := 0; at < len(value); at += step[at] {
		if matched[at] {
			out.WriteByte(' ')
		} else {
			out.WriteString(value[at : at+step[at]])
		}
	}
	return out.String()
}

// parseQuestion can protect names observed in already authorized candidates.
// Unknown parallel names remain subjects; unknown modifiers remain constraints.
// A zero-result query is never shortened.
func parseQuestion(question string, knownNames []string) questionSpec {
	normalized := normalizeCompoundRequests(question, knownNames)
	p := parseUnboundQuestion(normalized, knownNames)
	resolved, unresolved := resolveReferences(normalized, p, knownNames)
	if resolved != normalized {
		p = parseUnboundQuestion(resolved, knownNames)
	}
	bindQuestion(resolved, knownNames, &p)
	for _, name := range unresolved {
		p.Constraints = append(p.Constraints, "unresolved_reference:"+name)
	}
	return p
}

func parseUnboundQuestion(question string, knownNames []string) questionSpec {
	q := strings.TrimSpace(question)
	p := questionSpec{QuestionPlan: QuestionPlan{Requirements: []AttributeRequirement{}, Subjects: []string{}, Constraints: []string{}, Filters: []string{}, Scope: "literal"}, focus: q}
	if strings.ContainsAny(q, "\"“”") || strings.Contains(q, "技能") {
		return p
	}
	if !hasQuestionIntent(q, knownNames) {
		return p
	}
	var vocabulary attributeVocabulary
	for _, entry := range attributeWords() {
		for _, alias := range entry.aliases {
			if slices.Contains(fullHealthAliases(), alias) && hasOtherAttribute(q, entry.name) {
				continue
			}
			if entry.name == "再部署时间" && slices.Contains(redeploymentActions(), alias) && hasOtherAttribute(q, entry.name) {
				continue
			}
			if containsLanguage(q, alias) {
				vocabulary = entry
				break
			}
		}
		if vocabulary.name != "" {
			break
		}
	}
	if strings.Contains(q, "特性") {
		vocabulary = attributeVocabulary{"特性", []string{"特性"}}
	}
	if vocabulary.name == "" && strings.Contains(q, "落地") && strings.Contains(q, "费") {
		vocabulary = attributeVocabulary{"初始部署费用", []string{"落地", "费"}}
	}
	if vocabulary.name == "" {
		return p
	}
	p.natural = true
	p.Attribute = vocabulary.name
	p.focus = vocabulary.name
	if p.Attribute == "空中单位" {
		p.Attribute = "对空"
		p.focus = "对空"
		for _, priority := range []string{"优先", "先打", "先攻击", "先集火", "先对空"} {
			if strings.Contains(q, priority) {
				p.Attribute = "优先攻击空中单位"
				break
			}
		}
	}
	if p.Attribute == "阻挡数" {
		p.focus = "阻挡"
	}
	p.Scope = "named"
	p.Comparison = strings.Contains(q, "谁") || strings.Contains(q, "分别") || strings.Contains(q, "共同") || strings.Contains(q, "更")
	clean := q
	if p.Attribute == "所属势力" {
		for _, m := range membershipExpression.FindAllStringSubmatch(q, -1) {
			value := m[1]
			if value == "" {
				value = m[2]
			}
			value = strings.TrimRight(value, "吗呢呀？? ")
			if value == "" || strings.ContainsAny(value, "哪什么谁") {
				continue
			}
			p.Filters = append(p.Filters, value)
			clean = strings.ReplaceAll(clean, value, " ")
			p.focus += " " + value
		}
	}
	if stages := stageExpression.FindAllStringSubmatch(q, -1); len(stages) > 0 {
		for _, m := range stages {
			n, ok := smallNumber(m[1])
			if !ok {
				continue
			}
			stage := fmt.Sprintf("精英%d", n)
			if p.Stage != nil && *p.Stage != stage {
				p.Constraints = append(p.Constraints, "multiple_stages")
			}
			p.Stage = &stage
		}
		clean = stageExpression.ReplaceAllString(clean, " ")
	}
	if strings.Contains(q, "不含") || strings.Contains(q, "不包括") {
		clean = stripLanguage(clean, []string{"不含信赖和潜能", "不含信赖及潜能", "不包括信赖及潜能"})
	}
	if m := countExpression.FindStringSubmatch(clean); len(m) > 0 {
		p.expectedCount, _ = smallNumber(m[1])
		clean = countExpression.ReplaceAllString(clean, " $2 ")
	}
	// Protect authorized title aliases before removing grammatical particles.
	collectionHint := false
	for _, marker := range []string{"样本", "哪些", "这些", "所有", "全部"} {
		if strings.Contains(q, marker) {
			collectionHint = true
		}
	}
	aliases := slices.Clone(knownNames)
	slices.SortStableFunc(aliases, func(a, b string) int { return len([]rune(b)) - len([]rune(a)) })
	protected := map[string]string{}
	for i, name := range aliases {
		if name == "" || !strings.Contains(clean, name) {
			continue
		}
		key := fmt.Sprintf("zzsubject%czz", 'A'+i)
		clean = protectTitleAlias(clean, name, key)
		if strings.Contains(clean, key) {
			protected[key] = name
		}
	}
	// Unknown subject slots must also be protected before interpreting digits or
	// negative characters inside names as applicability constraints.
	for _, clause := range splitSubjectClauses(clean) {
		language := cleanRequestClause(clause)
		if containsRequestOperator(clean) {
			language = stripDisposalRequest(language, true)
		}
		name, _ := consumeUnknownSubject(language, clause, protected, collectionHint)
		if name != "" {
			key := fmt.Sprintf("zzunknown%dzz", len(protected))
			clean = protectTitleAlias(clean, name, key)
			protected[key] = name
		}
	}
	clean = neutralizePolarQuestions(clean)
	if strings.ContainsAny(clean, "不没无未") {
		p.Constraints = append(p.Constraints, "negation")
	}
	for _, span := range slices.Backward(numberExpression.FindAllStringIndex(clean, -1)) {
		if span[0] > 0 {
			b := clean[span[0]-1]
			if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' {
				continue
			}
		}
		p.Constraints = append(p.Constraints, "numeric:"+clean[span[0]:span[1]])
		clean = clean[:span[0]] + " " + clean[span[1]:]
	}
	requestContext := containsRequestOperator(clean)
	words := []string{
		"是不是", "是否", "进行", "会不会", "能不能", "不能", "不属于", "没有", "不在", "不含", "不包括",
		"初始", "普通攻击", "普通", "魔法", "物理", "手长", "打", "落地", "具体", "到底", "通常", "一般", "大概", "面板",
		"分别", "共同点", "共同", "相同", "相比", "比较", "对比", "更高", "更低", "更快", "更慢", "更短", "更长", "更大", "更小", "更多", "更少", "多挡一个敌人", "来比", "比", "贵", "便宜", "按",
		"精英等级", "满级", "一级", "1级", "不含信赖和潜能", "信赖", "潜能", "加成", "时", "阶段",
		"样本干员", "样本", "这些", "所有", "哪些", "干员", "哪家", "哪一关", "哪关", "主线教学", "主线", "教学", "关卡",
		"是多少", "多少个", "多少", "几个", "几多", "几成", "几", "什么类型", "什么", "哪一", "哪个", "怎样", "怎么", "如何", "多久", "多长时间", "多长", "时间", "间隔", "冷却",
		"优先", "先集火", "先", "攻击造成", "造成", "保留", "输出", "降低至", "降低到", "降到", "类型", "数值", "单位", "敌方", "敌人", "秒", "钟",
		"可以通过", "能通过", "可以", "通过", "才能", "得过", "过", "能", "会", "来一下", "需要", "要等", "都要", "等待", "等候", "等", "要", "想", "请问", "请", "帮我", "帮忙", "帮", "告诉我", "查", "一下", "包含", "包括",
		"一次", "一个", "正好", "得", "花", "隔", "还", "有", "里", "从", "在", "的", "是", "谁", "各自", "各", "逐一", "都", "吗", "啊", "呀", "呢", "呗", "么", "个",
	}
	if p.Attribute == "对空" || p.Attribute == "优先攻击空中单位" {
		words = append(words, "天上")
	}
	if p.Attribute == "阻挡数" {
		words = append(words, "怪物", "怪")
	}
	if p.Attribute == "初始部署费用" {
		words = append(words, "起手", "开场")
	}
	if p.Attribute == "生命上限" {
		words = append(words, "点")
	}
	if requestContext || p.Comparison {
		words = append(words, requestOrganizationWords()...)
	}
	for _, entry := range attributeWords() {
		for _, alias := range entry.aliases {
			if entry.name == "再部署时间" && p.Attribute != entry.name && slices.Contains(redeploymentActions(), alias) {
				continue
			}
			if !slices.Contains(fullHealthAliases(), alias) || p.Attribute == "生命上限" {
				words = append(words, alias)
			}
		}
	}
	words = append(words, phraseAliases([]string{"各", "各自", "分别"}, []string{"为", "是"})...)
	// A delimiter denotes a possible parallel subject; grammatical words do not.
	clauses := splitSubjectClauses(clean)
	for _, clause := range clauses {
		language := cleanRequestClause(clause)
		if requestContext {
			language = stripDisposalRequest(language, p.Attribute != "")
		}
		unknown, remainder := consumeUnknownSubject(language, clause, protected, collectionHint)
		language = remainder
		tokens := strings.FieldsFunc(stripLanguage(language, words), func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) })
		known := []string{}
		if unknown != "" {
			known = append(known, unknown)
		}
		residue := []string{}
		for _, token := range tokens {
			if name, ok := protected[token]; ok {
				known = append(known, name)
			} else {
				residue = append(residue, token)
			}
		}
		for _, name := range known {
			if !slices.Contains(p.Subjects, name) {
				p.Subjects = append(p.Subjects, name)
			}
		}
		modifier := len(known) > 0 || (collectionHint && len(protected) == 0) || isModifierClause(clause) || isRequestClause(clause)
		for _, part := range residue {
			if modifier {
				p.Constraints = append(p.Constraints, "unparsed_modifier:"+part)
			} else if !slices.Contains(p.Subjects, part) {
				p.Subjects = append(p.Subjects, part)
			}
		}
	}
	if len(p.Subjects) == 0 {
		p.Scope = "collection"
		if !collectionHint {
			p.Constraints = append(p.Constraints, "subject_unspecified")
		}
	}
	if len(p.Subjects) > 1 || strings.Contains(q, "贵") || strings.Contains(q, "便宜") {
		p.Comparison = true
	}
	if len(p.Subjects) > 8 {
		p.Constraints = append(p.Constraints, "subject_budget")
		p.Subjects = p.Subjects[:8]
	}
	if p.Attribute == "获得方式" {
		for _, term := range []string{"公开招募", "中坚寻访"} {
			if strings.Contains(q, term) {
				p.focus = term
				break
			}
		}
	}
	planRequirements(q, &p)
	return p
}

// A full-health state is a life amount shorthand only when no other requested
// attribute is present. Otherwise it remains an applicability condition.
func hasOtherAttribute(value, attribute string) bool {
	for _, entry := range attributeWords() {
		if entry.name == attribute {
			continue
		}
		for _, alias := range entry.aliases {
			if containsLanguage(value, alias) {
				return true
			}
		}
	}
	return false
}

func isModifierClause(value string) bool {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"按", "在", "谁", "哪个", "什么", "如何", "怎么", "是否"} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	// A postposed effect clause remains a condition even after its trailing
	// reporting action is removed; it cannot introduce a comparison subject.
	effect := trimLanguageTail(value, compiledRequestEnds)
	for _, suffix := range []string{"影响后", "影响前", "效果后", "效果前", "影响", "效果", "加成", "修正"} {
		if strings.HasSuffix(effect, suffix) {
			return true
		}
	}
	return false
}

func splitSubjectClauses(value string) []string {
	for _, separator := range []string{"以及", "还有", "跟", "和", "与", "及", "、", "，", ",", "；", ";"} {
		value = strings.ReplaceAll(value, separator, "\x00")
	}
	return strings.Split(value, "\x00")
}

func titleAlias(title string) string {
	for _, separator := range []string{"（", "("} {
		if i := strings.Index(title, separator); i > 0 {
			title = title[:i]
		}
	}
	return strings.TrimSpace(title)
}

package search

import (
	"slices"
	"unicode/utf8"
)

// Static language vocabulary is immutable after initialization. Accessors return
// capacity-limited views: callers may append without overwriting shared storage,
// but must clone before replacing elements or sorting. No page names or answers
// belong here. Builders retain the original phrase order and longest-first ties.

var compiledCourtesyPrefixes = buildCourtesyPrefixes()

func courtesyPrefixes() []string {
	return compiledCourtesyPrefixes[:len(compiledCourtesyPrefixes):len(compiledCourtesyPrefixes)]
}

func buildCourtesyPrefixes() []string {
	prefixes := append([]string{"麻烦", "劳驾", "烦请", "请问", "请", "帮忙"}, benefactivePrefixes()...)
	slices.SortStableFunc(prefixes, func(a, b string) int { return len(b) - len(a) })
	return prefixes
}

var compiledBenefactivePrefixes = buildBenefactivePrefixes()

func benefactivePrefixes() []string {
	return compiledBenefactivePrefixes[:len(compiledBenefactivePrefixes):len(compiledBenefactivePrefixes)]
}

func buildBenefactivePrefixes() []string {
	prefixes := phraseAliases([]string{"给", "替", "为", "帮"}, []string{"我们", "咱们", "你们", "我", "咱", "你", "您"})
	slices.SortStableFunc(prefixes, func(a, b string) int { return len(b) - len(a) })
	return prefixes
}

var compiledRequestVerbs = buildRequestVerbs()

func requestVerbs() []string {
	return compiledRequestVerbs[:len(compiledRequestVerbs):len(compiledRequestVerbs)]
}

func buildRequestVerbs() []string {
	return []string{"查", "说", "讲", "看", "瞅", "报", "列", "答", "说明", "解释", "排列", "核对", "核实", "核验", "核查", "确认", "查证", "查询", "查阅", "检索", "了解", "补充"}
}

func buildRequestOperators() []string {
	operators := []string{}
	for _, verb := range requestVerbs() {
		if len([]rune(verb)) > 1 {
			operators = append(operators, verb)
		}
		operators = append(operators, verb+verb, verb+"一下", verb+"一"+verb, verb+"下", verb+"出")
		operators = append(operators, phraseAliases([]string{verb}, requestCompletionComplements())...)
		for _, direction := range []string{"来", "去"} {
			operators = append(operators, verb+"出"+direction)
		}
		for _, recipient := range phraseAliases([]string{"给"}, []string{"我", "我们", "咱", "咱们", "你", "您", "你们"}) {
			operators = append(operators, verb+recipient, verb+recipient+"一下")
		}
	}
	// Degree complements modify the requested explanation, never a subject or
	// an attribute value. Keep them attached to their complete request action.
	complements := []string{"清楚", "明白"}
	clarity := phraseAliases([]string{"说", "讲", "报", "答", "核对", "核实", "核验", "核查", "确认", "查证"}, append(slices.Clone(complements), phraseAliases([]string{"得"}, complements)...))
	operators = append(operators, clarity...)
	operators = append(operators, phraseAliases(clarity, []string{"一点", "一些", "点", "些"})...)
	operators = append(operators, phraseAliases([]string{"告诉"}, []string{"我", "我们", "咱", "咱们", "你", "您", "你们"})...)
	operators = append(operators, phraseAliases([]string{"报", "说", "讲", "答", "给"}, phraseAliases([]string{"个"}, []string{"数值", "结果", "数"}))...)
	// Sequence markers are grammatical only when followed by a request action.
	// Output actions are already complete predicates, not directional stems.
	operators = append(operators, "给出", "提供", "返回")
	operators = append(operators, phraseAliases([]string{"再", "然后"}, slices.Clone(operators))...)
	slices.SortStableFunc(operators, func(a, b string) int { return len(b) - len(a) })
	return operators
}

var compiledRequestOrganizationWords = buildRequestOrganizationWords()

func requestOrganizationWords() []string {
	return compiledRequestOrganizationWords[:len(compiledRequestOrganizationWords):len(compiledRequestOrganizationWords)]
}

func buildRequestOrganizationWords() []string {
	return append([]string{"分别", "各自", "逐一", "逐个", "逐项", "逐条", "一并", "一起", "合起来", "全部", "对照着", "对照"}, phraseAliases(
		[]string{"按"}, []string{"人", "主体", "干员", "页面", "对象"})...)
}

func buildRequestDistribution() []string {
	units := []string{"人", "个人", "位", "主体", "个主体", "名干员", "个干员", "页面", "个页面", "对象", "个对象"}
	outputs := phraseAliases([]string{"一"}, []string{"条", "行", "项"})
	outputs = append(slices.Clone(outputs), phraseAliases([]string{"对应", "各"}, outputs)...)
	phrases := phraseAliases(phraseAliases([]string{"一", "每", "各", "逐"}, units), outputs)
	return append(append(slices.Clone(phrases), phraseAliases([]string{"按"}, phrases)...), "并排")
}

var compiledRequestAddressees = buildRequestAddressees()

func requestAddressees() []string {
	return compiledRequestAddressees[:len(compiledRequestAddressees):len(compiledRequestAddressees)]
}

func buildRequestAddressees() []string { return []string{"你们", "您", "你"} }

var compiledRequestAdverbs = buildRequestAdverbs()

func requestAdverbs() []string {
	return compiledRequestAdverbs[:len(compiledRequestAdverbs):len(compiledRequestAdverbs)]
}

func buildRequestAdverbs() []string {
	return []string{"直接", "简单", "简要", "简短", "简略", "大致", "分开", "顺便"}
}

func buildRequestPrefixes() []string {
	prefixes := append(append(courtesyPrefixes(), requestAddressees()...), requestAdverbs()...)
	prefixes = append(prefixes, phraseAliases([]string{"我", "我们", "咱们", ""}, []string{"想要", "想", "希望"})...)
	prefixes = append(prefixes, "先", "再", "然后", "随后", "接着", "接下来", "并且", "并", "继续")
	prefixes = append(append(prefixes, requestOrganizationWords()...), requestDistributionPhrases()...)
	slices.SortStableFunc(prefixes, func(a, b string) int { return len(b) - len(a) })
	return prefixes
}

var compiledRequestTails = buildRequestTails()

func requestTails() []string {
	return compiledRequestTails[:len(compiledRequestTails):len(compiledRequestTails)]
}

func buildRequestTails() []string {
	return append([]string{"谢谢", "好吗", "好么", "行吗", "可以吗", "吧", "呢", "呀", "啊", "呗", "嘛", "啦"}, phraseAliases([]string{"就"}, []string{"行", "好", "可以"})...)
}

var compiledPolarForms = buildPolarForms()

func polarForms() []string {
	return compiledPolarForms[:len(compiledPolarForms):len(compiledPolarForms)]
}

func buildPolarForms() []string {
	forms := []string{"有没有"}
	for _, predicate := range []string{"能", "会", "是", "需要", "要", "可以"} {
		_, size := utf8.DecodeRuneInString(predicate)
		forms = append(forms, predicate+"不"+predicate, predicate[:size]+"不"+predicate)
	}
	for _, predicate := range []string{"能", "可", "会", "是", "有", "需要", "可以"} {
		forms = append(forms, predicate+"否")
	}
	slices.SortStableFunc(forms, func(a, b string) int { return len(b) - len(a) })
	return forms
}

var compiledRequestOperators = buildRequestOperators()
var compiledRequestDistribution = buildRequestDistribution()
var compiledRequestPrefixes = buildRequestPrefixes()
var compiledRequestStarts = slices.Concat(courtesyPrefixes(), requestOperators())
var compiledRequestEnds = slices.Concat(requestTails(), requestOperators())
var compiledRequestNames = slices.Concat(requestOperators(), courtesyPrefixes(), requestVerbs(), requestDistributionPhrases(), requestAddressees(), requestAdverbs())

// Derived builders may mutate these copies; scanning reads compiled tables.
func requestOperators() []string {
	return slices.Clone(compiledRequestOperators)
}

func requestDistributionPhrases() []string {
	return slices.Clone(compiledRequestDistribution)
}

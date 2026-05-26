package goalexpr

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"corerp/internal/core"
)

type tokenType int

const (
	tokenEOF tokenType = iota
	tokenIdentifier
	tokenNumber
	tokenString
	tokenLParen
	tokenRParen
	tokenAnd
	tokenOr
	tokenNot
	tokenEq
	tokenNe
	tokenGt
	tokenGe
	tokenLt
	tokenLe
)

type token struct {
	typ tokenType
	raw string
	pos int
}

type parser struct {
	tokens []token
	pos    int
}

type boolNode interface {
	Eval(core.WorldState) bool
}

type valueNode interface {
	Eval(core.WorldState) interface{}
}

type binaryNode struct {
	op          tokenType
	left, right boolNode
}

type notNode struct {
	child boolNode
}

type comparisonNode struct {
	op          tokenType
	left, right valueNode
}

type valueBoolNode struct {
	value valueNode
}

type literalNode struct {
	value interface{}
}

type identifierNode struct {
	name string
}

func Validate(expr string) error {
	_, err := parse(expr)
	return err
}

func Eval(expr string, state core.WorldState) (bool, error) {
	if strings.TrimSpace(expr) == "" {
		return true, nil
	}
	root, err := parse(expr)
	if err != nil {
		return false, err
	}
	return root.Eval(state), nil
}

func parse(expr string) (boolNode, error) {
	tokens, err := tokenize(expr)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	node, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.peek().typ != tokenEOF {
		tok := p.peek()
		return nil, fmt.Errorf("unexpected token %q at position %d", tok.raw, tok.pos)
	}
	return node, nil
}

func tokenize(input string) ([]token, error) {
	var tokens []token
	for i := 0; i < len(input); {
		r := rune(input[i])
		if unicode.IsSpace(r) {
			i++
			continue
		}
		switch {
		case isIdentifierStart(r):
			start := i
			i++
			for i < len(input) && isIdentifierPart(rune(input[i])) {
				i++
			}
			raw := input[start:i]
			switch strings.ToUpper(raw) {
			case "AND":
				tokens = append(tokens, token{typ: tokenAnd, raw: raw, pos: start})
			case "OR":
				tokens = append(tokens, token{typ: tokenOr, raw: raw, pos: start})
			case "NOT":
				tokens = append(tokens, token{typ: tokenNot, raw: raw, pos: start})
			default:
				tokens = append(tokens, token{typ: tokenIdentifier, raw: raw, pos: start})
			}
		case unicode.IsDigit(r) || r == '-':
			start := i
			i++
			for i < len(input) && (unicode.IsDigit(rune(input[i])) || input[i] == '.') {
				i++
			}
			raw := input[start:i]
			if _, err := strconv.ParseFloat(raw, 64); err != nil {
				return nil, fmt.Errorf("invalid number %q at position %d", raw, start)
			}
			tokens = append(tokens, token{typ: tokenNumber, raw: raw, pos: start})
		case r == '"', r == '\'':
			quote := byte(r)
			start := i
			i++
			for i < len(input) && input[i] != quote {
				i++
			}
			if i >= len(input) {
				return nil, fmt.Errorf("unterminated string at position %d", start)
			}
			i++
			tokens = append(tokens, token{typ: tokenString, raw: input[start+1 : i-1], pos: start})
		case r == '(':
			tokens = append(tokens, token{typ: tokenLParen, raw: "(", pos: i})
			i++
		case r == ')':
			tokens = append(tokens, token{typ: tokenRParen, raw: ")", pos: i})
			i++
		case r == '=' && i+1 < len(input) && input[i+1] == '=':
			tokens = append(tokens, token{typ: tokenEq, raw: "==", pos: i})
			i += 2
		case r == '!' && i+1 < len(input) && input[i+1] == '=':
			tokens = append(tokens, token{typ: tokenNe, raw: "!=", pos: i})
			i += 2
		case r == '>' && i+1 < len(input) && input[i+1] == '=':
			tokens = append(tokens, token{typ: tokenGe, raw: ">=", pos: i})
			i += 2
		case r == '<' && i+1 < len(input) && input[i+1] == '=':
			tokens = append(tokens, token{typ: tokenLe, raw: "<=", pos: i})
			i += 2
		case r == '>':
			tokens = append(tokens, token{typ: tokenGt, raw: ">", pos: i})
			i++
		case r == '<':
			tokens = append(tokens, token{typ: tokenLt, raw: "<", pos: i})
			i++
		default:
			return nil, fmt.Errorf("invalid character %q at position %d", string(r), i)
		}
	}
	tokens = append(tokens, token{typ: tokenEOF, pos: len(input)})
	return tokens, nil
}

func isIdentifierStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentifierPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' || r == '-'
}

func (p *parser) parseExpr() (boolNode, error) {
	return p.parseOr()
}

func (p *parser) parseOr() (boolNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().typ == tokenOr {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: tokenOr, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (boolNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek().typ == tokenAnd {
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: tokenAnd, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseUnary() (boolNode, error) {
	if p.peek().typ == tokenNot {
		p.next()
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return notNode{child: child}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (boolNode, error) {
	if p.peek().typ == tokenLParen {
		p.next()
		node, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peek().typ != tokenRParen {
			tok := p.peek()
			return nil, fmt.Errorf("expected ')' at position %d", tok.pos)
		}
		p.next()
		return node, nil
	}
	left, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	switch p.peek().typ {
	case tokenEq, tokenNe, tokenGt, tokenGe, tokenLt, tokenLe:
		op := p.next().typ
		right, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		return comparisonNode{op: op, left: left, right: right}, nil
	default:
		return valueBoolNode{value: left}, nil
	}
}

func (p *parser) parseValue() (valueNode, error) {
	tok := p.next()
	switch tok.typ {
	case tokenIdentifier:
		lower := strings.ToLower(tok.raw)
		switch lower {
		case "true":
			return literalNode{value: true}, nil
		case "false":
			return literalNode{value: false}, nil
		case "always", "active":
			return literalNode{value: true}, nil
		case "never":
			return literalNode{value: false}, nil
		default:
			return identifierNode{name: tok.raw}, nil
		}
	case tokenNumber:
		v, _ := strconv.ParseFloat(tok.raw, 64)
		return literalNode{value: v}, nil
	case tokenString:
		return literalNode{value: tok.raw}, nil
	default:
		return nil, fmt.Errorf("unexpected token %q at position %d", tok.raw, tok.pos)
	}
}

func (p *parser) peek() token {
	return p.tokens[p.pos]
}

func (p *parser) next() token {
	tok := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return tok
}

func (n binaryNode) Eval(state core.WorldState) bool {
	switch n.op {
	case tokenAnd:
		return n.left.Eval(state) && n.right.Eval(state)
	case tokenOr:
		return n.left.Eval(state) || n.right.Eval(state)
	default:
		return false
	}
}

func (n notNode) Eval(state core.WorldState) bool {
	return !n.child.Eval(state)
}

func (n comparisonNode) Eval(state core.WorldState) bool {
	left := n.left.Eval(state)
	right := n.right.Eval(state)
	if lf, lok := asFloat(left); lok {
		if rf, rok := asFloat(right); rok {
			switch n.op {
			case tokenEq:
				return lf == rf
			case tokenNe:
				return lf != rf
			case tokenGt:
				return lf > rf
			case tokenGe:
				return lf >= rf
			case tokenLt:
				return lf < rf
			case tokenLe:
				return lf <= rf
			}
		}
	}
	switch n.op {
	case tokenEq:
		return normalize(left) == normalize(right)
	case tokenNe:
		return normalize(left) != normalize(right)
	case tokenGt:
		return normalize(left) > normalize(right)
	case tokenGe:
		return normalize(left) >= normalize(right)
	case tokenLt:
		return normalize(left) < normalize(right)
	case tokenLe:
		return normalize(left) <= normalize(right)
	default:
		return false
	}
}

func (n valueBoolNode) Eval(state core.WorldState) bool {
	return truthy(n.value.Eval(state))
}

func (n literalNode) Eval(core.WorldState) interface{} {
	return n.value
}

func (n identifierNode) Eval(state core.WorldState) interface{} {
	if value, ok := lookupStateValue(state, n.name); ok {
		return value
	}
	return n.name
}

func lookupStateValue(state core.WorldState, key string) (interface{}, bool) {
	if value, ok := lookupBuiltin(state, key); ok {
		return value, true
	}
	if value, ok := lookupRelationship(state, key); ok {
		return value, true
	}
	if strings.HasPrefix(key, "flags.") {
		v, ok := state.Flags[strings.TrimPrefix(key, "flags.")]
		return v, ok
	}
	if strings.HasPrefix(key, "variables.") {
		v, ok := lookupVariable(state.Variables, strings.TrimPrefix(key, "variables."))
		return v, ok
	}
	if v, ok := state.Flags[key]; ok {
		return v, true
	}
	return lookupVariable(state.Variables, key)
}

func lookupBuiltin(state core.WorldState, key string) (interface{}, bool) {
	switch key {
	case "scene", "scene.location":
		return state.Scene.Location, true
	case "time_of_day", "scene.time_of_day":
		return state.Scene.TimeOfDay, true
	case "weather", "scene.weather":
		return state.Scene.Weather, true
	case "tension":
		return state.Tension, true
	case "day":
		return float64(state.Clock.Day), true
	case "hour":
		return float64(state.Clock.Hour), true
	case "minute":
		return float64(state.Clock.Minute), true
	default:
		return nil, false
	}
}

func lookupRelationship(state core.WorldState, key string) (interface{}, bool) {
	if strings.HasPrefix(key, "relationships.") {
		path := strings.TrimPrefix(key, "relationships.")
		lastDot := strings.LastIndex(path, ".")
		if lastDot <= 0 {
			return nil, false
		}
		relKey := path[:lastDot]
		field := path[lastDot+1:]
		rel, ok := state.Relationships[relKey]
		if !ok {
			return nil, false
		}
		return relationshipField(rel, field)
	}
	if strings.HasPrefix(key, "relationship.") {
		field := strings.TrimPrefix(key, "relationship.")
		if len(state.Relationships) == 0 {
			return nil, false
		}
		for _, rel := range state.Relationships {
			if value, ok := relationshipField(rel, field); ok {
				return value, true
			}
		}
	}
	return nil, false
}

func relationshipField(rel core.Relationship, field string) (interface{}, bool) {
	switch field {
	case "trust":
		return rel.Trust, true
	case "intimacy":
		return rel.Intimacy, true
	case "fear":
		return rel.Fear, true
	case "respect":
		return rel.Respect, true
	case "debt":
		return rel.Debt, true
	case "last_scene":
		return rel.LastScene, true
	default:
		return nil, false
	}
}

func lookupVariable(vars map[string]interface{}, key string) (interface{}, bool) {
	if vars == nil {
		return nil, false
	}
	if v, ok := vars[key]; ok {
		return v, true
	}
	parts := strings.Split(key, ".")
	var current interface{} = vars
	for _, part := range parts {
		nextMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = nextMap[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func truthy(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case float32:
		return t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	case string:
		lower := strings.ToLower(strings.TrimSpace(t))
		return lower != "" && lower != "false" && lower != "0" && lower != "never"
	default:
		return v != nil
	}
}

func asFloat(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func normalize(v interface{}) string {
	switch t := v.(type) {
	case bool:
		if t {
			return "true"
		}
		return "false"
	case string:
		return strings.ToLower(strings.TrimSpace(t))
	default:
		return strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
	}
}

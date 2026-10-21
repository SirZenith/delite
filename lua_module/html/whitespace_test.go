package html_test

import "testing"

func TestRemoveRedundantWhitespaceKeepsInlineSeparator(t *testing.T) {
	err := addModuleAndDo(`
		local html = require "html"
		local atom = require "html-atom"

		local Node = html.Node

		local p = Node.new_element(atom.P)
		p:append_child(Node.new_text("For government sales inquiries, please contact "))
		local a = Node.new_element(atom.A)
		a:append_child(Node.new_text("governmentsales@pearsoned.com"))
		p:append_child(a)
		p:append_child(Node.new_text("."))

		p:remove_redundant_whitespace()

		local text = p:extract_all_text()
		assert(text == "For government sales inquiries, please contact governmentsales@pearsoned.com.",
			"space before inline element was lost: " .. text)
	`)

	handleError(t, "failed to keep the space before an inline element", err)
}

func TestRemoveRedundantWhitespaceKeepsWhitespaceOnlySeparator(t *testing.T) {
	err := addModuleAndDo(`
		local html = require "html"
		local atom = require "html-atom"

		local Node = html.Node

		local p = Node.new_element(atom.P)
		p:append_child(Node.new_text("alpha "))

		local b = Node.new_element(atom.B)
		b:append_child(Node.new_text("one"))
		p:append_child(b)

		p:append_child(Node.new_text("   "))

		local i = Node.new_element(atom.I)
		i:append_child(Node.new_text("two"))
		p:append_child(i)

		p:append_child(Node.new_text(" omega"))

		p:remove_redundant_whitespace()

		local text = p:extract_all_text()
		assert(text == "alpha one two omega",
			"whitespace-only separator between inline elements was lost: " .. text)
	`)

	handleError(t, "failed to keep the whitespace-only separator", err)
}

func TestRemoveRedundantWhitespaceTrimsBlockBoundaries(t *testing.T) {
	err := addModuleAndDo(`
		local html = require "html"
		local atom = require "html-atom"

		local Node = html.Node

		local p = Node.new_element(atom.P)
		p:append_child(Node.new_text("  \n  foo  \n  "))

		p:remove_redundant_whitespace()

		local text = p:extract_all_text()
		assert(text == "foo", "block boundary whitespace was not trimmed: " .. text)
	`)

	handleError(t, "failed to trim block boundary whitespace", err)
}

func TestRemoveRedundantWhitespaceKeepsInlineElementEdges(t *testing.T) {
	err := addModuleAndDo(`
		local html = require "html"
		local atom = require "html-atom"

		local Node = html.Node

		local b = Node.new_element(atom.B)
		b:append_child(Node.new_text("  bar  "))

		b:remove_redundant_whitespace()

		local text = b:extract_all_text()
		assert(text == " bar ", "inline element edge whitespace was trimmed: " .. text)
	`)

	handleError(t, "failed to keep inline element edge whitespace", err)
}

func TestRemoveRedundantWhitespaceSkipAttr(t *testing.T) {
	err := addModuleAndDo(`
		local html = require "html"
		local atom = require "html-atom"

		local Node = html.Node

		local p = Node.new_element(atom.P)
		p:attr("delite-no-trim-space", "1")
		p:append_child(Node.new_text("  foo  "))

		p:remove_redundant_whitespace()

		local text = p:extract_all_text()
		assert(text == "  foo  ", "delite-no-trim-space was ignored: " .. text)
	`)

	handleError(t, "failed to honor delite-no-trim-space", err)
}

func TestRemoveRedundantWhitespaceSkipsLiteralContent(t *testing.T) {
	err := addModuleAndDo(`
		local html = require "html"
		local atom = require "html-atom"

		local Node = html.Node

		local pre = Node.new_element(atom.Pre)
		pre:append_child(Node.new_text("  foo  "))

		pre:remove_redundant_whitespace()

		local text = pre:extract_all_text()
		assert(text == "  foo  ", "preformatted whitespace was modified: " .. text)

		local code = Node.new_element(atom.Code)
		code:append_child(Node.new_text("  bar  "))

		code:remove_redundant_whitespace()

		text = code:extract_all_text()
		assert(text == "  bar  ", "code whitespace was modified: " .. text)
	`)

	handleError(t, "failed to skip preformatted or code content", err)
}

func TestRemoveRedundantWhitespaceKeepsNonBreakingSpaces(t *testing.T) {
	err := addModuleAndDo(`
		local html = require "html"
		local atom = require "html-atom"

		local Node = html.Node

		local p = Node.new_element(atom.P)
		p:append_child(Node.new_text(string.char(194, 160)))
		p:remove_redundant_whitespace()
		assert(p:extract_all_text() == string.char(194, 160),
			"non-breaking space was removed")

		local ideographic = Node.new_element(atom.P)
		ideographic:append_child(Node.new_text(string.char(227, 128, 128)))
		ideographic:remove_redundant_whitespace()
		assert(ideographic:extract_all_text() == string.char(227, 128, 128),
			"ideographic space was removed")
	`)

	handleError(t, "failed to keep non-breaking or ideographic spaces", err)
}

func TestRemoveRedundantWhitespaceTreatsBrAsBlockBoundary(t *testing.T) {
	err := addModuleAndDo(`
		local html = require "html"
		local atom = require "html-atom"

		local Node = html.Node

		local p = Node.new_element(atom.P)
		p:append_child(Node.new_text("foo "))
		p:append_child(Node.new_element(atom.Br))
		p:append_child(Node.new_text(" bar"))

		p:remove_redundant_whitespace()

		local text = p:extract_all_text()
		assert(text == "foobar", "br boundary whitespace was not trimmed: " .. text)
	`)

	handleError(t, "failed to treat br as a block boundary", err)
}

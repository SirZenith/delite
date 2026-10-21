package converter_test

import (
	"testing"

	"github.com/SirZenith/delite/lua_module/docconv/converter"
	"github.com/SirZenith/delite/lua_module/docconv/linked_list"
	"github.com/SirZenith/delite/lua_module/html"
	"github.com/SirZenith/delite/lua_module/html/atom"
	lua "github.com/yuin/gopher-lua"
)

func addModulesAndDo(code string) error {
	L := lua.NewState()
	defer L.Close()
	L.PreloadModule("html", html.Loader)
	L.PreloadModule("html-atom", atom.Loader)
	L.PreloadModule("docconv.converter", converter.Loader)
	L.PreloadModule("docconv.linked_list", linked_list.Loader)
	return L.DoString(code)
}

func handleError(t *testing.T, msg string, err error) {
	if err == nil {
		return
	}
	t.Fatalf("%s:\n%s", msg, err)
}

func TestTrimSpaceRemovesWhitespaceOnlyEdges(t *testing.T) {
	err := addModulesAndDo(`
		local html = require "html"
		local atom = require "html-atom"
		local converter = require "docconv.converter"
		local linked_list = require "docconv.linked_list"

		local node = html.Node.new_element(atom.P)
		local content = linked_list.List.new()
		content:append("  ", " foo", " ", "bar ", "  ")

		local result = converter.trim_space(node, "", content)
		local parts = result:totable()

		assert(#parts == 3, "unexpected element count: " .. #parts)
		assert(parts[1] == "foo", "leading text was not trimmed: " .. tostring(parts[1]))
		assert(parts[2] == " ", "inner separator was removed: " .. tostring(parts[2]))
		assert(parts[3] == "bar", "trailing text was not trimmed: " .. tostring(parts[3]))
	`)

	handleError(t, "trim_space did not remove whitespace-only edges", err)
}

func TestTrimSpaceHonorsSkipAttr(t *testing.T) {
	err := addModulesAndDo(`
		local html = require "html"
		local atom = require "html-atom"
		local converter = require "docconv.converter"
		local linked_list = require "docconv.linked_list"

		local node = html.Node.new_element(atom.P)
		node:attr("delite-no-trim-space", "1")
		local content = linked_list.List.new()
		content:append("  keep  ")

		local result = converter.trim_space(node, "", content)
		local text = table.concat(result:totable(), "")

		assert(text == "  keep  ", "skip attr was ignored: " .. string.format("%q", text))
	`)

	handleError(t, "trim_space ignored the skip attr", err)
}

func TestReplaceMultipleSpacePreservesTrailingSpace(t *testing.T) {
	err := addModulesAndDo(`
		local html = require "html"
		local atom = require "html-atom"
		local converter = require "docconv.converter"
		local linked_list = require "docconv.linked_list"

		local node = html.Node.new_element(atom.P)
		local content = linked_list.List.new()
		content:append("foo ")

		local result = converter.replace_multiple_space(node, "", content)
		local text = table.concat(result:totable(), "")

		assert(text == "foo ", "trailing space was dropped: " .. string.format("%q", text))
	`)

	handleError(t, "replace_multiple_space dropped the trailing space", err)
}

func TestReplaceMultipleSpaceDoesNotDuplicateCrossElementSpace(t *testing.T) {
	err := addModulesAndDo(`
		local html = require "html"
		local atom = require "html-atom"
		local converter = require "docconv.converter"
		local linked_list = require "docconv.linked_list"

		local node = html.Node.new_element(atom.P)
		local content = linked_list.List.new()
		content:append(" ", "foo", " ", " bar ")

		local result = converter.replace_multiple_space(node, "", content)
		local text = table.concat(result:totable(), "")

		assert(text == " foo bar ", "cross-element spaces were duplicated: " .. string.format("%q", text))
	`)

	handleError(t, "replace_multiple_space duplicated a cross-element space", err)
}

func TestReplaceMultipleSpacePreservesLeadingSpace(t *testing.T) {
	err := addModulesAndDo(`
		local html = require "html"
		local atom = require "html-atom"
		local converter = require "docconv.converter"
		local linked_list = require "docconv.linked_list"

		local node = html.Node.new_element(atom.P)
		local content = linked_list.List.new()
		content:append(" foo")

		local result = converter.replace_multiple_space(node, "", content)
		local text = table.concat(result:totable(), "")

		assert(text == " foo", "leading space was dropped: " .. string.format("%q", text))
	`)

	handleError(t, "replace_multiple_space dropped the leading space", err)
}

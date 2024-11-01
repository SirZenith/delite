package html_test

import (
	"testing"

	"github.com/SirZenith/delite/lua_module/html"
	"github.com/SirZenith/delite/lua_module/html/atom"
	lua "github.com/yuin/gopher-lua"
)

func addModuleAndDo(code string) error {
	L := lua.NewState()
	L.PreloadModule("html", html.Loader)
	L.PreloadModule("html-atom", atom.Loader)
	return L.DoString(code)
}

func handleError(t *testing.T, msg string, err error) {
	if err == nil {
		return
	}
	t.Fatalf("%s:\n%s", msg, err)
}

func TestImport(t *testing.T) {
	err := addModuleAndDo(`
		local html = require "html"
		local atom = require "html-atom"
	`)

	handleError(t, "failed to import html module", err)
}

func TestAtomTbl(t *testing.T) {
	err := addModuleAndDo(`
		local atom = require "html-atom"

		for k, v in pairs(atom) do
			print(k, v)
		end
	`)

	handleError(t, "failed to access Atom table", err)
}

func TestAtomToString(t *testing.T) {
	err := addModuleAndDo(`
		local atom = require "html-atom"
		print(atom.tostring(atom.A))
	`)

	handleError(t, "failed to convert atom to string", err)
}

func TestNewNode(t *testing.T) {
	err := addModuleAndDo(`
		local html = require "html"
		local atom = require "html-atom"

		local Node = html.Node

		local nodes = {
			Node.new(),
			Node.new_text("hello"),
			Node.new_doc(),
			Node.new_element(atom.P),
			Node.new_comment("comment content"),
			Node.new_doctype("html"),
			Node.new_raw("<foo></foo>"),
		}

		for _, node in ipairs(nodes) do
			print(node)
		end
	`)

	handleError(t, "failed to create node", err)
}

func TestNodeChild(t *testing.T) {
	err := addModuleAndDo(`
		local html = require "html"
		local atom = require "html-atom"

		local Node = html.Node

		local root = Node.new_doc()
		local p_tag = Node.new_element(atom.P)
		local text = Node.new_text("hello")

		root:append_child(p_tag)
		p_tag:append_child(text)

		print(root:first_child() == p_tag:first_child())
		print(p_tag:first_child() == p_tag:last_child())

		for child in p_tag:iter_children() do
			print(child)
		end

		p_tag:remove_child(text)
		print(p_tag:first_child() == nil)
	`)

	handleError(t, "failed to test node equality", err)
}

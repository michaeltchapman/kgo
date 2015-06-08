package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
)

// Macros we can handle and understand
var knownMacros = [...]string{".Nm", ".Op", ".Ar", ".Fl"}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

type Command struct {
	name     string
	syntaxes []Syntax
}

type Syntax struct {
	parameters []Parameter
}

type Parameter struct {
	name         string
	optional     bool
	nospace      bool
	hasargument  bool
	argument     string
	hasflags     bool
	flags        string
	hasparameter bool
	parameter    *Parameter
}

func main() {
	parseManFiles("/usr/share/man/man1", 0, 0)
}

func getFileList(path string) []string {
	filepaths := []string{}
	fileinfos, err := ioutil.ReadDir(path)

	if err != nil {
		fmt.Println("Failed to read directory %s", path)
	}

	for _, file := range fileinfos {
		if !(file.IsDir()) {
			filepaths = append(filepaths, path+"/"+file.Name())
		}
	}
	return filepaths
}

func parseManFiles(path string, rangeLower int, rangeUpper int) {
	files := getFileList(path)

	var s []string
	if rangeUpper == 0 && rangeLower == 0 {
		s = files[:]
	} else {
		s = files[rangeLower:rangeUpper]
	}

	//for _, file := range files[495:496] { // login debugging
	for _, file := range s {
		command, err := manfileToCommand(file)
		if err != nil {
			continue
		} else {
			fmt.Println(file)
			fmt.Println(command)
		}
	}
}

func manfileToCommand(path string) (Command, error) {
	rawlines := loadFileToLines(path)
	lines := getSynopsisLines(rawlines)
	name := getDefinedName(rawlines)
	command, err := buildCommand(name, lines)
	return command, err
}

func loadFileToLines(path string) []string {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Println("Failed to read file at path: %s", path)
	}
	return strings.Split(string(data), "\n")
}

func quoteString(s string) string {
	return "\"" + s + "\""
}

func pass(s string) string {
	return s
}

// Determine if this is a synopsis heading. Some will be quoted/captialised
func isSynopsisLine(line string) bool {
	modfunctions := []func(string) string{quoteString, pass, strings.ToUpper}
	ret := false
	for _, shfunc := range modfunctions {
		for _, synfunc := range modfunctions {
			ret = ret || strings.HasPrefix(line, (shfunc(".Sh")+" "+synfunc("synopsis")))
		}
	}
	return ret
}

// Some man pages will define their name and use .Nm as shorthand
func getDefinedName(lines []string) string {
	re := regexp.MustCompile("^\\.Nm ([a-z]+)$")
	for _, line := range lines {
		result := re.FindAllStringSubmatch(line, -1)
		if len(result) > 0 {
			return strings.Split(result[0][0], " ")[1]
		}
	}
	return ""
}

func isNameLine(line string) bool {
	re := regexp.MustCompile("^\\.Nm( \\w+)?")
	return re.MatchString(line)
}

// Get all the lines below the synopsis heading
func getSynopsisLines(lines []string) [][]string {
	start := 0
	synopsis := [][]string{}
	usagePattern := -1

	for i, line := range lines {
		// Find the start of the synopsis section which contains the arguments
		if isSynopsisLine(line) {
			start = i
			continue
		}
		// Add lines until we reach the next section
		if start != 0 {
			if !(strings.HasPrefix(line, ".Sh") || strings.HasPrefix(line, ".SH")) {
				if compliantLine(line) {
					// Usually a name line is at the start, but a couple don't do this.
					// The command is printed regardless, eg rlogin
					if isNameLine(line) || usagePattern == -1 {
						synopsis = append(synopsis, []string{})
						usagePattern++
					}
					synopsis[usagePattern] = append(synopsis[usagePattern], line)
				}
			} else {
				break
			}
		}
	}
	return synopsis
}

// Many synopsis sections are done using bold/italics rather than macros
// Let's ignore them because who knows what the author was thinking
func compliantLine(line string) bool {
	for _, macro := range knownMacros {
		if strings.Contains(line, macro) {
			return true
		}
	}
	return false
}

func buildCommand(name string, paramLines [][]string) (Command, error) {
	syntax := []Syntax{}
	var err error
	err = nil
	for _, lineset := range paramLines {
		syn, e := buildSyntax(lineset)
		if e != nil {
			err = e
		} else if isValidSyntax(syn) {
			syntax = append(syntax, syn)
		}
	}
	if len(syntax) == 0 {
		err = errors.New("No syntaxes found")
	}
	return Command{name: name, syntaxes: syntax}, err
}

func buildSyntax(lines []string) (Syntax, error) {
	parameters := []Parameter{}
	var err error
	err = nil
	for _, line := range lines {
		param, e := buildParameter(strings.Split(line, " "))
		if e != nil {
			err = e
		} else if isValidParameter(param) {
			parameters = append(parameters, param)
		}
	}
	return Syntax{parameters: parameters}, err
}

// Convert a string to an array of Parameters. The aggregate of these
// will form a Syntax and the set of Syntaxes forms a command. Most
// lines will only be a single parameter
func buildParameter(tokens []string) (Parameter, error) {
	p := Parameter{}
	var err error
	err = nil
	for i, rawtoken := range tokens {
		token := strings.TrimLeft(rawtoken, ".")
		if token == "Op" {
			if !p.optional {
				p.optional = true
			} else if !p.hasparameter {
				p.hasparameter = true
				tp, e := buildParameter(tokens[i:])
				if err != nil {
					err = e
				} else {
					p.parameter = &tp
				}
			}
		}

		if token == "Ar" && !p.hasargument {
			if !p.hasargument {
				p.hasargument = true
				// if the next token is blank, it's a generic non-named argument
				if len(tokens) > i+1 {
					p.argument = tokens[i+1]
				} else {
					p.argument = "files"
				}
			} else if !p.hasparameter {
				p.hasparameter = true
				tp, e := buildParameter(tokens[i:])
				if err != nil {
					err = e
				} else {
					p.parameter = &tp
				}
			}
		}

		if token == "Fl" && !p.hasflags {
			if !p.hasflags {
				p.hasflags = true
				if len(tokens) > i+1 {
					p.flags = tokens[i+1]
				} else {
					p.flags = "-"
				}
			} else if !p.hasparameter {
				p.hasparameter = true
				tp, e := buildParameter(tokens[i:])
				if err != nil {
					err = e
				} else {
					p.parameter = &tp
				}
			}
		}
	}
	return p, err
}

func prependDashes(s string) string {
	lines := strings.Split(s, "\n")
	out := ""
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			out = out + "--" + line + "\n"
		}
	}
	return out
}

func isValidParameter(p Parameter) bool {
	return (p.optional || p.nospace || p.hasflags || p.hasargument || p.hasparameter)
}

func isValidSyntax(s Syntax) bool {
	return (len(s.parameters) > 0)
}

func (c Command) String() string {
	ret := fmt.Sprintf("Command: %s\n", c.name)
	for _, syn := range c.syntaxes {
		ret = ret + prependDashes(syn.String()) + "\n"
	}
	return ret
}

func (s Syntax) String() string {
	ret := ""
	for _, param := range s.parameters {
		ret = ret + param.String() + "\n"
	}
	return ret
}

func (p Parameter) String() string {
	ret := ""
	if p.optional {
		ret = ret + "--optional\n"
	}
	if p.nospace {
		ret = ret + "--nospace\n"
	}
	if p.hasflags {
		ret = ret + fmt.Sprintf("--flags: %s\n", p.flags)
	}
	if p.hasargument {
		ret = ret + "--has argument: " + p.argument + "\n"
	}
	if p.hasparameter {
		ret = ret + "--has nested parameter:\n" + prependDashes(p.parameter.String())
	}
	if ret != "" {
		ret = "Parameter:\n" + ret
	}
	return ret
}

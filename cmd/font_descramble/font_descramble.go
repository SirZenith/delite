package font_descramble

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"slices"
	"sort"

	"github.com/urfave/cli/v3"
	"golang.org/x/image/font/sfnt"
)

const defaultPPEM = 15

func Cmd() *cli.Command {
	var standard string
	var scrambled string

	cmd := &cli.Command{
		Name:    "font-descramble",
		Aliases: []string{"fd"},
		Usage:   "compares two SFNT font file, and prints mapping, which maps source rune to target rune that has the same glyph in standard font as source rune has in scrambled font",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "job",
				Aliases: []string{"j"},
				Value:   int64(runtime.NumCPU()),
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "standard",
				UsageText:   "<standard-file>",
				Min:         1,
				Max:         1,
				Destination: &standard,
			},
			&cli.StringArg{
				Name:        "scrambled",
				UsageText:   "<scrambled-file>",
				Min:         1,
				Max:         1,
				Destination: &scrambled,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cmdMain(cmd, standard, scrambled)
		},
	}

	return cmd
}

func cmdMain(cmd *cli.Command, standardFile, scrambledFile string) error {
	jobCnt := cmd.Int("job")

	ctx, err := newMatchContext(standardFile, scrambledFile, int(jobCnt))
	if err != nil {
		return fmt.Errorf("initialization failed: %s", err)
	}

	pairs, err := ctx.getTranslateMap()
	if err != nil {
		return fmt.Errorf("translation failed: %s", err)
	}

	if len(pairs) == 0 {
		fmt.Println("no matching rune found")
		return nil
	}

	for _, pair := range pairs {
		fmt.Printf("'%s': '%s',\n", string(pair.scmRune), string(pair.stdRune))
	}

	return nil
}

// Loads font font disk.
func loadFont(fileName string) (*sfnt.Font, error) {
	data, err := os.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("cannot open file %s: %s", fileName, err)
	}

	font, err := sfnt.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse font data: %s", err)
	}

	return font, nil
}

// Takes a worker function and generates a map value with it concurrently.
// Worker function should return `struct { key key; value value; isFinished bool }`,
// `key` and `value` will be store to final map as a pair.
// And before worker function ends its workd and return, it must send a struct
// with `isFinished` set to `true` through result channel.
// Main goroutine will wait until all worker goroutine signal a `isFinished`,
// if any of your workers ended without sending `isFinished` message, program
// will be locked forever.
func fontInfoMapMaker[key comparable, value any](
	font *sfnt.Font,
	name string,
	maxValue int,
	jobCnt int,
	workerFunc func(
		*sfnt.Font,
		chan struct {
			key        key
			value      value
			isFinished bool
		},
		int,
		int,
	),
) map[key]value {
	result := map[key]value{}
	resultChan := make(chan struct {
		key        key
		value      value
		isFinished bool
	}, jobCnt*2)

	stepLen := int(maxValue) / jobCnt
	for i := 0; i < maxValue; i += stepLen {
		ed := i + stepLen
		if ed > maxValue {
			ed = maxValue
		}

		go workerFunc(font, resultChan, i, ed)
	}

	count := 0
	finishedCnt := 0
	writer := log.Writer()
	fmt.Fprintf(writer, "setup %s %10d", name, count)
	for data := range resultChan {
		if data.isFinished {
			finishedCnt++
		} else {
			result[data.key] = data.value
			count++

			if count%100 == 0 {
				fmt.Fprintf(writer, "\rsetup %s %10d", name, count)
			}
		}

		if finishedCnt >= jobCnt {
			break
		}
	}
	fmt.Fprintf(writer, "\rsetup %s          \u2713\n", name)

	return result
}

// Worker function for making glyph index to glyph drawing argumetn map.
func makeGlyphArgsMapWorker(
	font *sfnt.Font,
	resultChan chan struct {
		key        sfnt.GlyphIndex
		value      []int32
		isFinished bool
	},
	rangeSt,
	rangeEd int,
) {
	buffer := &sfnt.Buffer{}

	for i := rangeSt; i < rangeEd; i++ {
		index := sfnt.GlyphIndex(i)
		glyph, err := font.LoadGlyph(buffer, index, defaultPPEM, nil)
		if err != nil {
			continue
		}

		if len(glyph) == 0 {
			continue
		}

		args := []int32{}
		for _, seg := range glyph {
			args = append(args, int32(seg.Op))
			for _, point := range seg.Args {
				args = append(args, int32(point.X))
				args = append(args, int32(point.Y))
			}
		}

		resultChan <- struct {
			key        sfnt.GlyphIndex
			value      []int32
			isFinished bool
		}{
			key:   index,
			value: args,
		}
	}
	resultChan <- struct {
		key        sfnt.GlyphIndex
		value      []int32
		isFinished bool
	}{
		isFinished: true,
	}
}

// Worker function for making rune to glyph index map.
func makeRuneIndexMapWorker(
	font *sfnt.Font,
	resultChan chan struct {
		key        rune
		value      sfnt.GlyphIndex
		isFinished bool
	},
	rangeSt,
	rangeEd int,
) {
	buffer := &sfnt.Buffer{}

	for i := rangeSt; i < rangeEd; i++ {
		target := rune(i)
		index, err := font.GlyphIndex(buffer, target)
		if err != nil || index == 0 {
			continue
		}

		resultChan <- struct {
			key        rune
			value      sfnt.GlyphIndex
			isFinished bool
		}{
			key:   target,
			value: index,
		}
	}

	resultChan <- struct {
		key        rune
		value      sfnt.GlyphIndex
		isFinished bool
	}{
		isFinished: true,
	}
}

// A struct used to do generate rune translation map.
type matchContext struct {
	stdFont, scmFont *sfnt.Font

	stdArgsMap  map[sfnt.GlyphIndex][]int32 // maps standard glyph index to glyph arguments
	stdRuneMap  map[rune]sfnt.GlyphIndex    // maps runes to standard glyph index
	stdIndexMap map[sfnt.GlyphIndex]rune    // maps standard glyph index to standard rune

	scmArgsMap  map[sfnt.GlyphIndex][]int32         // maps scrambled glyph index to glyph arguments
	scmRuneMap  map[rune]sfnt.GlyphIndex            // maps runes to scrambled glyph index
	scmIndexMap map[sfnt.GlyphIndex]sfnt.GlyphIndex // maps scrambled glyph index to standard glyph index
}

func newMatchContext(standardFile, scrambledFile string, jobCnt int) (*matchContext, error) {
	standardFont, err := loadFont(standardFile)
	if err != nil {
		return nil, err
	}

	scrambledFont, err := loadFont(scrambledFile)
	if err != nil {
		return nil, err
	}

	ctx := &matchContext{
		stdFont: standardFont,
		scmFont: scrambledFont,
	}

	ctx.initStdIndexMap(jobCnt)
	ctx.initScrambledIndexMap(jobCnt)

	return ctx, nil
}

// Initializes info maps for standard font. This method is called in `newMatchContext`
// function.
func (c *matchContext) initStdIndexMap(jobCnt int) {
	c.stdArgsMap = fontInfoMapMaker(c.stdFont, "standard args", c.stdFont.NumGlyphs(), jobCnt, makeGlyphArgsMapWorker)
	c.stdRuneMap = fontInfoMapMaker(c.stdFont, "standard rune", math.MaxInt32, jobCnt, makeRuneIndexMapWorker)

	indexMap := map[sfnt.GlyphIndex]rune{}
	for rune, index := range c.stdRuneMap {
		indexMap[index] = rune
	}
	c.stdIndexMap = indexMap
}

type glyphMatchWorkLoad struct {
	key   sfnt.GlyphIndex
	value sfnt.GlyphIndex
}

// Initializes info maps for scrambled font. This method is called in `newMatchContext`
// function.
func (c *matchContext) initScrambledIndexMap(jobCnt int) {
	c.scmArgsMap = fontInfoMapMaker(c.scmFont, "scrambled args", c.scmFont.NumGlyphs(), jobCnt, makeGlyphArgsMapWorker)
	c.scmRuneMap = fontInfoMapMaker(c.scmFont, "scrambled rune", math.MaxInt32, jobCnt, makeRuneIndexMapWorker)

	taskChan := make(chan sfnt.GlyphIndex, jobCnt)
	resultChan := make(chan glyphMatchWorkLoad, jobCnt)

	for i := 0; i < jobCnt; i++ {
		go c.findMatchingGlyphIndex(taskChan, resultChan)
	}

	go func() {
		for scmIndex := range c.scmArgsMap {
			taskChan <- scmIndex
		}
		close(taskChan)
	}()

	count := 0
	finishedCnt := 0
	indexMap := map[sfnt.GlyphIndex]sfnt.GlyphIndex{}
	c.scmIndexMap = indexMap

	writer := log.Writer()
	fmt.Fprintf(writer, "matched glyphs %10d", count)
	for matched := range resultChan {
		if matched.value == 0 {
			finishedCnt++
		} else {
			indexMap[matched.key] = matched.value
			count++

			if count%100 == 0 {
				fmt.Fprintf(writer, "\rmatched glyphs %10d", count)
			}
		}

		if finishedCnt >= jobCnt {
			break
		}
	}
	fmt.Fprintf(writer, "\rmatched glyphs %10d\n", count)
}

// Worker functions for finding corresponding glyph index of a glyph in scrambled
// font by matching each standard glyph's drawing arguments with target glyph.
// Target glyph is read from `taskChan`, and matching result will be send to
// `resultChan`.
func (c *matchContext) findMatchingGlyphIndex(taskChan chan sfnt.GlyphIndex, resultChan chan glyphMatchWorkLoad) {
	for scm := range taskChan {
		scmArgs := c.scmArgsMap[scm]

		var matched sfnt.GlyphIndex
		for stdIndex, stdArgs := range c.stdArgsMap {
			if slices.Equal(scmArgs, stdArgs) {
				matched = stdIndex
				break
			}
		}

		if matched > 0 {
			resultChan <- glyphMatchWorkLoad{
				key:   scm,
				value: matched,
			}
		}
	}

	resultChan <- glyphMatchWorkLoad{}
}

type runePair struct {
	scmRune rune
	stdRune rune
}

// Returns a map which maps scrambled rune to standard rune value.
func (c *matchContext) getTranslateMap() ([]runePair, error) {
	result := []runePair{}

	keys := []int{}
	for k := range c.scmRuneMap {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)

	for _, key := range keys {
		scmRune := rune(key)
		scmIndex := c.scmRuneMap[scmRune]
		stdIndex, ok := c.scmIndexMap[scmIndex]
		if !ok {
			continue
		}

		if stdRune, ok := c.stdIndexMap[stdIndex]; ok {
			result = append(result, runePair{scmRune: scmRune, stdRune: stdRune})
		}
	}

	return result, nil
}

// Returns drawing arguments in scrambled font for given rune. If no glyph is
// found for given rune, an empty argument list is returned.
func (c *matchContext) getScmArgsForRune(target rune) []int32 {
	var scmArgs []int32
	if scmIndex, ok := c.scmRuneMap[target]; ok {
		scmArgs = c.scmArgsMap[scmIndex]
	}

	return scmArgs
}

// Returns drawing arguments in standard font for given rune. If no glyph is
// found for given rune, an empty argument list is returned.
func (c *matchContext) getStdArgsForRune(target rune) []int32 {
	var stdArgs []int32
	if stdIndex, ok := c.stdRuneMap[target]; ok {
		stdArgs = c.stdArgsMap[stdIndex]
	}

	return stdArgs
}

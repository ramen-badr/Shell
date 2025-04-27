package parser

import (
	"bufio"
	"fmt"
	"shell/internal/execute"
	"shell/internal/slice"
	"slices"
	"strings"
)

func Read(s *bufio.Scanner) []byte {
	var line []byte

	for s.Scan() || line != nil {
		var quote byte

		if line == nil {
			line = s.Bytes()
		} else {
			line = append(line, s.Bytes()...)
		}

		for i := 0; i < len(line); i++ {
			switch {
			case line[i] == '\\' && (quote == 0 || (quote == '"' && i+1 < len(line) && (line[i+1] == '"' || line[i+1] == '\\'))):
				i++
			case (line[i] == '\'' || line[i] == '"') && quote == 0:
				quote = line[i]
			case line[i] == quote:
				quote = 0
			}
		}

		if len(line) >= 1 && line[len(line)-1] == '\\' {
			if quote != '\'' {
				line = slice.Remove(line, len(line)-1, len(line))
			}
			fmt.Print("> ")
			continue
		}

		if quote != 0 {
			fmt.Print("> ")
			continue
		}

		break
	}

	return line
}

func QuotesHandle(line []byte, id int) (string, int) {
	var res strings.Builder

	for quote := byte(0); id < len(line) && (quote != 0 || !slices.Contains([]byte{' ', '|', '&', '<', '>', ';'}, line[id])); id++ {
		switch {
		case line[id] == '\\' && (quote == 0 || (quote == '"' && id+1 < len(line) && (line[id+1] == '"' || line[id+1] == '\\'))):
			id++
		case (line[id] == '\'' || line[id] == '"') && quote == 0:
			quote = line[id]
			continue
		case line[id] == quote:
			quote = 0
			continue
		}

		res.WriteByte(line[id])
	}

	return res.String(), id
}

func Parse(line []byte) []exec.Command {
	var res []exec.Command
	var cmd exec.Command
	var str string
	var appendFlag bool

	for i := 0; i < len(line); i++ {
		i = slice.TrimSpaces(line, i)
		if i == len(line) {
			break
		}

		switch line[i] {
		case '&':
			if len(cmd.CmdArgs) == 0 {
				fmt.Println("Syntax error: missing command before '&'")
				return nil
			}

			cmd.Background = true
			res = append(res, cmd)
			cmd = exec.Command{}
		case '|':
			if len(cmd.CmdArgs) == 0 {
				fmt.Println("Syntax error: missing command before '|'")
				return nil
			}

			cmd.CmdFlag += 2
			res = append(res, cmd)
			cmd = exec.Command{CmdFlag: 1}
		case '<':
			i = slice.TrimSpaces(line, i+1)
			if i == len(line) {
				fmt.Println("Syntax error: missing input file name after '<'")
				return nil
			}

			cmd.InFile, i = QuotesHandle(line, i)
		case '>':
			if i+1 < len(line) && line[i+1] == '>' {
				appendFlag = true
				i++
			}

			i = slice.TrimSpaces(line, i+1)
			if i == len(line) {
				fmt.Println("Syntax error: missing output file name after '>'")
				return nil
			}

			if appendFlag {
				cmd.AppFile, i = QuotesHandle(line, i)
			} else {
				cmd.OutFile, i = QuotesHandle(line, i)
			}
		case ';':
			if len(cmd.CmdArgs) == 0 {
				fmt.Println("Syntax error: missing command before ';'")
				return nil
			}

			res = append(res, cmd)
			cmd = exec.Command{}
		default:
			str, i = QuotesHandle(line, i)
			cmd.CmdArgs = append(cmd.CmdArgs, str)
		}
	}

	if len(cmd.CmdArgs) != 0 {
		res = append(res, cmd)
	}

	return res
}

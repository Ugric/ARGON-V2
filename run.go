package main

import (
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

var runop func(codeseg any, origin string, vargroups []map[string]variableValue) (any, any)

func init() {
	runop = runprocess
}

func run(lines []any, origin string, vargroups []map[string]variableValue) (any, any, [][]any) {
	output := ([][]any{})
	for i := 0; i < len(lines); i++ {
		if lines[i] != nil {
			val, ty := runprocess(lines[i], origin, vargroups)
			output = append(output, []any{val, ty})
			if ty == "return" || ty == "break" || ty == "continue" || ty == "error" {
				return ty, val, output
			}
		}
	}
	return nil, nil, output
}

func anyToArgon(x any, quote bool) string {
	switch x := x.(type) {
	case string:
		if !quote {
			return x
		} else {
			return strconv.Quote(x)
		}
	case float64:
		if math.IsNaN(x) {
			return "NaN"
		} else if math.IsInf(x, 1) {
			return "infinity"
		} else if math.IsInf(x, -1) {
			return "-infinity"
		} else {
			return strconv.FormatFloat(x, 'f', -1, 64)
		}
	case bool:
		if x {
			return "yes"
		} else {
			return "no"
		}
	case nil:
		return "unknown"
	case []any:
		output := []string{}
		for i := 0; i < len(x); i++ {
			output = append(output, anyToArgon(x[i], true))
		}
		return "[" + strings.Join(output, ", ") + "]"
	case variable:
		return "<function " + x.variable + ">"
	default:
		return fmt.Sprint(x)
	}
}

func runprocess(codeseg any, origin string, vargroups []map[string]variableValue) (any, any) {
	switch codeseg := codeseg.(type) {
	case opperator:
		resp, ty := runOperator(codeseg, vargroups, origin)
		return resp, ty
	case variable:
		var varigroup map[string]variableValue
		for i := len(vargroups) - 1; i >= 0; i-- {
			if vargroups[i][codeseg.variable].EXISTS != nil {
				varigroup = vargroups[i]
				break
			}
		}
		myvar := varigroup[codeseg.variable]
		if myvar.EXISTS == nil {
			myvar = vargroups[len(vargroups)-1][codeseg.variable]
		}
		if myvar.EXISTS != nil {
			if myvar.TYPE != "func" && myvar.TYPE != "init_function" {
				return myvar.VAL, "value"
			} else {
				return codeseg, "value"
			}
		}
		return ("undecared variable " + codeseg.variable + ": " + origin + ":" + fmt.Sprint(codeseg.line+1)), "error"
	case funcCallType:
		resp, ty := callFunc(codeseg, vargroups, origin)
		return resp, ty
	case errorType:
		resp, _ := runprocess(codeseg.val, origin, vargroups)
		return resp, "error"
	case itemsType:
		vals := []any{}
		for i := 0; i < len(codeseg.vals); i++ {
			resp, err := runprocess(codeseg.vals[i], origin, vargroups)
			if err != nil {
				return "invalid value: " + origin + ":" + fmt.Sprint(codeseg.line+1), "error"
			}
			vals = append(vals, resp)
		}
		return vals, "value"
	case tryType:

		ty, resp, _ := run(codeseg.code, origin, append(vargroups, map[string]variableValue{}))
		if ty == "error" {
			ty, resp, _ := run(codeseg.catch, origin, append(vargroups, map[string]variableValue{"err": {
				TYPE:   "var",
				EXISTS: true,
				VAL:    resp,
				FUNC:   false,
			}}))
			return resp, ty
		}
		return resp, ty
	case whileLoop:
		whileloop := codeseg
		resp, ty := runop(whileloop.condition, origin, vargroups)
		if ty == "error" {
			return resp, ty
		}
		vari := append(vargroups, map[string]variableValue{})
		for boolean(resp) {
			ty, val, _ := run(whileloop.code, origin, vari)
			if ty == "break" {
				break
			} else if ty == "continue" {
				continue
			} else if ty == "return" || ty == "error" {
				return val, ty
			}
			resp, ty = runop(whileloop.condition, origin, vargroups)
			if ty == "error" {
				return resp, ty
			}
		}
		return nil, nil
	case ifstatement:
		vari := append(vargroups, map[string]variableValue{})
		iff := codeseg
		for i := 0; i < len(iff.statments); i++ {
			resp, ty := runop(iff.statments[i].condition, origin, vargroups)
			if ty == "error" {
				return resp, ty
			}
			if boolean(resp) {
				ty, val, _ := run(iff.statments[i].code, origin, vari)
				return val, ty
			}
		}
		ty, val, _ := run(iff.FALSE, origin, vari)
		return val, ty
	case importType:
		resp, ty := runop(codeseg.path, origin, vargroups)
		if ty == "error" {
			return resp, ty
		}
		importvars, err := importMod(resp.(string), filepath.Dir(origin))
		if err != nil {
			return err, "error"
		}
		if codeseg.toImport == nil {
			for name, val := range importvars {
				vargroups[len(vargroups)-1][name] = val
			}
		} else {
			var toImport = codeseg.toImport.([]string)
			for i := 0; i < len(toImport); i++ {
				vargroups[len(vargroups)-1][toImport[i]] = importvars[toImport[i]]
			}
		}
		return nil, nil
	case setVariable:
		value := setVariableVal(codeseg, vargroups, origin)
		if value == nil {
			return nil, nil
		} else {
			return value, "error"
		}
	case setFunction:
		value := setFunctionVal(codeseg, vargroups, origin)
		if value == nil {
			return nil, nil
		} else {
			return value, "error"
		}
	case returnType:
		var val any = nil
		if codeseg.val != nil {
			vars, worked := runop(codeseg.val, origin, vargroups)
			if worked == nil {
				return "return statement must return a value: " + origin + ":" + fmt.Sprint(codeseg.line+1), "error"
			}
			val = vars
		}
		return val, "return"
	case breakType:
		return nil, "break"
	case continueType:
		return nil, "continue"
	}
	return codeseg, "value"
}

func callFunc(call funcCallType, vargroups []map[string]variableValue, origin string) (any, any) {
	var variables map[string]variableValue
	for i := len(vargroups) - 1; i >= 0; i-- {
		if vargroups[i][call.name.variable].EXISTS != nil {
			variables = vargroups[i]
			break
		}
	}

	if variables[call.name.variable].EXISTS == nil {
		return ("undecared function '" + call.name.variable + "': " + origin + ":" + fmt.Sprint(call.line+1)), "error"
	}
	callable := call
	for variables[callable.name.variable].TYPE != "func" && variables[callable.name.variable].TYPE != "init_function" && fmt.Sprint(reflect.TypeOf(variables[callable.name.variable].VAL)) == "main.variable" {
		callable = funcCallType{name: variables[callable.name.variable].VAL.(variable), args: callable.args, line: callable.line}
	}
	for i := len(vargroups) - 1; i >= 0; i-- {
		if vargroups[i][callable.name.variable].EXISTS != nil {
			variables = vargroups[i]
			break
		}
	}
	if variables[callable.name.variable].TYPE != "func" && variables[callable.name.variable].TYPE != "init_function" {
		return ("'" + call.name.variable + "' is not a function: " + origin + ":" + fmt.Sprint(call.line+1)), "error"
	}
	if variables[callable.name.variable].FUNC {

		argvals := []any{}
		for i := 0; i < len(callable.args); i++ {
			resp, ty := runprocess(callable.args[i], origin, vargroups)
			if ty == "error" {
				return resp, ty
			}
			argvals = append(argvals, resp)
		}
		val, err := variables[callable.name.variable].VAL.(func(...any) (any, any))(argvals...)
		if err != nil {
			return err, "error"
		}
		return val, "value"
	} else {
		argvars := make(map[string]variableValue)
		for i := 0; i < len(callable.args); i++ {
			resp, ty := runprocess(callable.args[i], origin, vargroups)
			if ty == "error" {
				return resp, ty
			}
			name := variables[callable.name.variable].VAL.(setFunction).args[i]
			argvars[name] = variableValue{
				VAL:    resp,
				TYPE:   "var",
				EXISTS: true,
				FUNC:   false,
			}
		}
		ty, val, _ := run(variables[callable.name.variable].VAL.(setFunction).code, origin, append(vargroups, modules[variables[callable.name.variable].origin], argvars))
		if ty != "return" && ty != "error" && ty != nil {
			return fmt.Sprint(ty) + " is not allowed in function: " + origin + ":" + fmt.Sprint(callable.line+1), "error"
		}
		return val, "value"
	}
}

func setVariableVal(x setVariable, vargroups [](map[string]variableValue), origin string) any {
	var variable map[string]variableValue = nil
	if x.TYPE == "preset" {
		for i := len(vargroups) - 1; i >= 0; i-- {
			if vargroups[i][x.variable.variable].EXISTS != nil {
				variable = vargroups[i]
				break
			}
		}
	} else {
		variable = vargroups[len(vargroups)-1]
	}
	if variable[x.variable.variable].EXISTS == nil {
		resp, ty := runop(x.value, origin, vargroups)
		if ty == "error" {
			return resp
		}
		var TYPE = x.TYPE
		if TYPE == "preset" {
			TYPE = "var"
		}
		vargroups[len(vargroups)-1][x.variable.variable] = variableValue{
			TYPE:   TYPE,
			EXISTS: true,
			VAL:    resp,
			origin: origin,
			FUNC:   false,
		}
	} else if variable[x.variable.variable].TYPE == "var" {
		resp, ty := runop(x.value, origin, vargroups)
		if ty == "error" {
			return resp
		}
		var TYPE = x.TYPE
		if TYPE == "preset" {
			TYPE = "var"
		}
		variable[x.variable.variable] = variableValue{
			TYPE:   TYPE,
			EXISTS: variable[x.variable.variable].EXISTS,
			VAL:    resp,
			origin: origin,
			FUNC:   variable[x.variable.variable].FUNC,
		}
	} else {
		return ("cannot edit " + variable[x.variable.variable].TYPE + " variable: " + origin + ":" + fmt.Sprint(x.line+1))
	}
	return nil
}

func setFunctionVal(x setFunction, vargroups []map[string]variableValue, origin string) any {
	var variable map[string]variableValue
	for i := len(vargroups) - 1; i >= 0; i-- {
		if vargroups[i][x.name].EXISTS != nil {
			variable = vargroups[i]
			break
		}
	}
	if variable[x.name].EXISTS == nil {
		vargroups[len(vargroups)-1][x.name] = variableValue{
			TYPE:   "func",
			EXISTS: true,
			VAL:    x,
			FUNC:   false,
			origin: origin,
		}
	} else if variable[x.name].TYPE == "func" {
		vari := variable[x.name]
		vari.VAL = x
		vari.FUNC = false
	} else {
		return ("cannot edit " + variable[x.name].TYPE + " variable: " + origin + ":" + fmt.Sprint(x.line+1))
	}
	return nil
}

func dynamicAdd(x any, y any) (any, any) {
	stringconvert := false
	switch x.(type) {
	case string:
		stringconvert = true
	}
	switch y.(type) {
	case string:
		stringconvert = true
	}
	if stringconvert {
		return fmt.Sprint(x) + fmt.Sprint(y), nil
	}
	xnum, err := number(x)
	if err != nil {
		return err, "error"
	}
	ynum, err := number(y)
	if err != nil {
		return err, "error"
	}
	return xnum + ynum, nil
}

func xiny(x any, y []any) bool {
	for i := 0; i < len(y); i++ {
		if x == y[i] {
			return true
		}
	}
	return false
}

func boolean(x any) bool {
	return (x != false && x != nil && x != 0 && x != "")
}

func number(x any) (float64, any) {
	switch x := x.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case bool:
		if x {
			return 1, nil
		} else {
			return 0, nil
		}
	default:
		num, err := strconv.ParseFloat(fmt.Sprint(x), 64)
		if err != nil {
			return 0, (err)
		}
		return num, nil
	}
}

func runOperator(opperation opperator, vargroups []map[string]variableValue, origin string) (any, any) {
	var output any
opperationloop:
	for i := 0; i < len(opperation.vals); i++ {
		switch opperation.t {
		case 0:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				if boolean(output) && boolean(x) {
					output = x
				} else {
					output = false
					break opperationloop
				}
			}
		case 1:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if boolean(x) {
				output = x
				break opperationloop
			}
		case 2:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				output = xiny(output, x.([]any))
			}
		case 3:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				output = !xiny(output, x.([]interface{}))
			}
		case 4:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				out, err := number(output)
				if err != nil {
					return 0, (err)
				}

				in, err := number(x)
				if err != nil {
					return 0, (err)
				}
				output = (out <= in)
			}
		case 5:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				out, err := number(output)
				if err != nil {
					return 0, (err)
				}

				in, err := number(x)
				if err != nil {
					return 0, (err)
				}
				output = (out >= in)
			}
		case 6:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				out, err := number(output)
				if err != nil {
					return 0, (err)
				}

				in, err := number(x)
				if err != nil {
					return 0, (err)
				}
				output = (out < in)
			}
		case 7:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				out, err := number(output)
				if err != nil {
					return 0, (err)
				}

				in, err := number(x)
				if err != nil {
					return 0, (err)
				}
				output = (out > in)
			}
		case 8:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				output = (output != x)
			}
		case 9:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				output = (output == x)
			}
		case 11:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				out, err := number(output)
				if err != nil {
					return 0, (err)
				}

				in, err := number(x)
				if err != nil {
					return 0, (err)
				}
				output = (out - in)
			}
		case 10:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				val, err := dynamicAdd(output, x)
				if err != nil {
					return 0, (err)
				}
				output = val
			}
		case 12:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				out, err := number(output)
				if err != nil {
					return 0, (err)
				}

				in, err := number(x)
				if err != nil {
					return 0, (err)
				}
				output = (out * in)
			}
		case 14:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				out, err := number(output)
				if err != nil {
					return 0, (err)
				}

				in, err := number(x)
				if err != nil {
					return 0, (err)
				}
				output = math.Floor(out / in)
			}
		case 13:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				out, err := number(output)
				if err != nil {
					return 0, (err)
				}

				in, err := number(x)
				if err != nil {
					return 0, (err)
				}
				output = math.Mod(out, in)
			}
		case 15:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				out, err := number(output)
				if err != nil {
					return 0, (err)
				}

				in, err := number(x)
				if err != nil {
					return 0, (err)
				}
				output = (out / in)
			}
		case 16:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				out, err := number(output)
				if err != nil {
					return 0, (err)
				}

				in, err := number(x)
				if err != nil {
					return 0, (err)
				}
				output = math.Pow(out, 1/in)
			}
		case 17:
			x, ty := runop(opperation.vals[i], origin, vargroups)
			if ty == "error" {
				return x, ty
			}
			if output == nil {
				output = x
			} else {
				out, err := number(output)
				if err != nil {
					return 0, (err)
				}

				in, err := number(x)
				if err != nil {
					return 0, (err)
				}
				output = math.Pow(out, in)
			}
		}
	}
	return output, "opperator"
}

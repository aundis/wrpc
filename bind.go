package wrpc

import (
	"context"
	"reflect"
	"strings"

	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/text/gstr"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/gogf/gf/v2/util/gvalid"
)

func (c *Client) call(ctx context.Context, name string, params ...interface{}) (interface{}, error) {
	var (
		ok          bool
		err         error
		inputValues = []reflect.Value{
			reflect.ValueOf(ctx),
		}
	)
	funcInfo := c.handlerMap[name]
	if funcInfo == nil {
		return nil, gerror.NewCodef(gcode.CodeNotFound, `not found handler '%s'`, name)
	}

	if funcInfo.Type.NumIn() == 2 {
		if len(params) == 0 {
			return nil, gerror.NewCodef(gcode.CodeMissingParameter, `missing parameter for handler '%s'`, name)
		}

		var (
			err         error
			inputObject reflect.Value
			param       = params[0]
		)
		if funcInfo.Type.In(1).Kind() == reflect.Ptr {
			inputObject = reflect.New(funcInfo.Type.In(1).Elem())
			err = c.doParse(ctx, inputObject.Interface(), param)
		} else {
			inputObject = reflect.New(funcInfo.Type.In(1).Elem()).Elem()
			err = c.doParse(ctx, inputObject.Addr().Interface(), param)
		}
		if err != nil {
			return nil, err
		}
		inputValues = append(inputValues, inputObject)
	}
	// Call handler with dynamic created parameter values.
	results := funcInfo.Value.Call(inputValues)
	switch len(results) {
	case 1:
		if !results[0].IsNil() {
			if err, ok = results[0].Interface().(error); ok {
				return nil, err
			}
		}
		return nil, nil

	case 2:
		if !results[1].IsNil() {
			if err, ok = results[1].Interface().(error); ok {
				return nil, err
			}
		}
		return results[0].Interface(), nil
	}

	return nil, gerror.NewCodef(gcode.CodeInternalError, `can't run to here`)
}

func (c *Client) doParse(ctx context.Context, pointer interface{}, params interface{}) (err error) {
	var (
		reflectVal1  = reflect.ValueOf(pointer)
		reflectKind1 = reflectVal1.Kind()
	)
	if reflectKind1 != reflect.Ptr {
		return gerror.NewCodef(
			gcode.CodeInvalidParameter,
			`invalid parameter type "%v", of which kind should be of *struct/**struct/*[]struct/*[]*struct, but got: "%v"`,
			reflectVal1.Type(),
			reflectKind1,
		)
	}

	// Converting.
	err = gconv.Struct(params, pointer)
	if err != nil {
		return err
	}

	// TODO: https://github.com/gogf/gf/pull/2450
	// Validation.
	if err = gvalid.New().
		Bail().
		Data(pointer).
		Assoc(params).
		Run(ctx); err != nil {
		return err
	}

	return nil
}

func (c *Client) MustBind(name string, fn interface{}) {
	if err := c.Bind(name, fn); err != nil {
		panic(err)
	}
}

func (c *Client) Bind(name string, fn interface{}) error {
	funcInfo, err := c.checkAndCreateFuncInfo(name, fn)
	if err != nil {
		return err
	}
	c.handlerMap[name] = &funcInfo
	return nil
}

type handlerFuncInfo struct {
	Type  reflect.Type  // Reflect type information for current handler, which is used for extensions of the handler feature.
	Value reflect.Value // Reflect value information for current handler, which is used for extensions of the handler feature.
}

func (c *Client) checkAndCreateFuncInfo(name string, fn interface{}) (funcInfo handlerFuncInfo, err error) {
	funcInfo = handlerFuncInfo{
		Type:  reflect.TypeOf(fn),
		Value: reflect.ValueOf(fn),
	}

	method := name
	reflectType := funcInfo.Type
	if reflectType.NumIn() == 0 || reflectType.NumIn() > 2 {
		err = gerror.NewCodef(
			gcode.CodeInvalidParameter,
			`invalid bind handler: %s defined as "%s", but the input parameter number should be 1 or 2`, method, reflectType.String(),
		)
		return
	}
	if reflectType.NumOut() == 0 || reflectType.NumOut() > 2 {
		err = gerror.NewCodef(
			gcode.CodeInvalidParameter,
			`invalid bind handler: %s defined as "%s", but the output parameter number should be 1 or 2`, method, reflectType.String(),
		)
		return
	}

	if !reflectType.In(0).Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
		err = gerror.NewCodef(
			gcode.CodeInvalidParameter,
			`invalid handler: defined as "%s", but the first input parameter should be type of "context.Context"`,
			reflectType.String(),
		)
		return
	}

	errorIndex := reflectType.NumOut() - 1
	if !reflectType.Out(errorIndex).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		err = gerror.NewCodef(
			gcode.CodeInvalidParameter,
			`invalid handler: defined as "%s", but the last output parameter should be type of "error"`,
			reflectType.String(),
		)
		return
	}

	hasInputReq := reflectType.NumIn() == 2
	if hasInputReq && (reflectType.In(1).Kind() != reflect.Ptr ||
		(reflectType.In(1).Kind() == reflect.Ptr && reflectType.In(1).Elem().Kind() != reflect.Struct)) {
		err = gerror.NewCodef(
			gcode.CodeInvalidParameter,
			`invalid handler: defined as "%s", but the second input parameter should be type of pointer to struct like "*BizReq"`,
			reflectType.String(),
		)
		return
	}

	// The request struct should be named as `xxxReq`.
	if hasInputReq {
		reqStructName := trimGeneric(reflectType.In(1).String())
		if !gstr.HasSuffix(reqStructName, `Req`) {
			err = gerror.NewCodef(
				gcode.CodeInvalidParameter,
				`invalid struct naming for request: defined as "%s", but it should be named with "Req" suffix like "XxxReq"`,
				reqStructName,
			)
			return
		}
	}

	// The response struct should be named as `xxxRes`.
	// resStructName := trimGeneric(reflectType.Out(0).String())
	// if !gstr.HasSuffix(resStructName, `Res`) {
	// 	err = gerror.NewCodef(
	// 		gcode.CodeInvalidParameter,
	// 		`invalid struct naming for response: defined as "%s", but it should be named with "Res" suffix like "XxxRes"`,
	// 		resStructName,
	// 	)
	// 	return
	// }

	// funcInfo.IsStrictRoute = true

	// inputObject = reflect.New(funcInfo.Type.In(1).Elem())
	// inputObjectPtr = inputObject.Interface()

	// It retrieves and returns the request struct fields.
	// fields, err := gstructs.Fields(gstructs.FieldsInput{
	// 	Pointer:         inputObjectPtr,
	// 	RecursiveOption: gstructs.RecursiveOptionEmbedded,
	// })
	// if err != nil {
	// 	return funcInfo, err
	// }
	// funcInfo.ReqStructFields = fields
	// funcInfo.Func = createRouterFunc(funcInfo)
	return
}

func trimGeneric(structName string) string {
	var (
		leftBraceIndex  = strings.LastIndex(structName, "[") // for generic, it is faster to start at the end than at the beginning
		rightBraceIndex = strings.LastIndex(structName, "]")
	)
	if leftBraceIndex == -1 || rightBraceIndex == -1 {
		// not found '[' or ']'
		return structName
	} else if leftBraceIndex+1 == rightBraceIndex {
		// may be a slice, because generic is '[X]', not '[]'
		// to be compatible with bad return parameter type: []XxxRes
		return structName
	}
	return structName[:leftBraceIndex]
}

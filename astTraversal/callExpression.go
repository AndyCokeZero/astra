package astTraversal

import (
	"go/ast"
	"go/types"
)

type CallExpressionTraverser struct {
	Traverser *BaseTraverser
	Node      *ast.CallExpr
	File      *FileNode
}

func (t *BaseTraverser) CallExpression(node ast.Node) (*CallExpressionTraverser, error) {
	callExpr, ok := node.(*ast.CallExpr)
	if !ok {
		return nil, ErrInvalidNodeType
	}

	return &CallExpressionTraverser{
		Traverser: t,
		Node:      callExpr,
		File:      t.ActiveFile(),
	}, nil
}

func (c *CallExpressionTraverser) Function() (*FunctionTraverser, error) {
	if c != nil && c.Traverser != nil && c.Node != nil {
		if cached, ok := c.Traverser.callExprFuncCache[c.Node]; ok {
			return cached, nil
		}
		if cachedErr, ok := c.Traverser.callExprFuncErrCache[c.Node]; ok {
			return nil, cachedErr
		}
	}

	decl, err := c.Traverser.FindDeclarationForNode(c.Node.Fun)
	if err != nil {
		if c != nil && c.Traverser != nil && c.Node != nil {
			c.Traverser.callExprFuncErrCache[c.Node] = err
		}
		return nil, err
	}

	function, err := c.Traverser.Function(decl.Decl)
	if err != nil {
		if c != nil && c.Traverser != nil && c.Node != nil {
			c.Traverser.callExprFuncErrCache[c.Node] = err
		}
		return nil, err
	}

	if c != nil && c.Traverser != nil && c.Node != nil {
		c.Traverser.callExprFuncCache[c.Node] = function
	}

	return function, nil
}

func (c *CallExpressionTraverser) ArgIndex(argName string) (int, bool) {
	for i, arg := range c.Node.Args {
		ident, ok := arg.(*ast.Ident)
		if !ok {
			continue
		}

		if ident.Name == argName {
			return i, true
		}
	}

	return 0, false
}

func (c *CallExpressionTraverser) Args() []ast.Expr {
	return c.Node.Args
}

func (c *CallExpressionTraverser) Type() (*types.Func, error) {
	if c.Node.Fun == nil {
		return nil, ErrInvalidNodeType
	}

	if c != nil && c.Traverser != nil && c.Node != nil {
		if cached, ok := c.Traverser.callExprTypeCache[c.Node]; ok {
			return cached, nil
		}
		if cachedErr, ok := c.Traverser.callExprTypeErrCache[c.Node]; ok {
			return nil, cachedErr
		}
	}

	var obj types.Object
	var err error
	switch nodeFun := c.Node.Fun.(type) {
	case *ast.Ident:
		obj, err = c.File.Package.FindObjectForIdent(nodeFun)
	case *ast.SelectorExpr:
		obj, err = c.File.Package.FindUsesForIdent(nodeFun.Sel)
		if err != nil {
			obj, err = c.File.Package.FindObjectForIdent(nodeFun.Sel)
		}
	default:
		err = ErrInvalidNodeType
	}
	if err != nil {
		if c != nil && c.Traverser != nil && c.Node != nil {
			c.Traverser.callExprTypeErrCache[c.Node] = err
		}
		return nil, err
	}

	switch objType := obj.(type) {
	case *types.Func:
		if c != nil && c.Traverser != nil && c.Node != nil {
			c.Traverser.callExprTypeCache[c.Node] = objType
		}
		return objType, nil
	case *types.Builtin:
		if c != nil && c.Traverser != nil && c.Node != nil {
			c.Traverser.callExprTypeErrCache[c.Node] = ErrBuiltInFunction
		}
		return nil, ErrBuiltInFunction
	}

	if c != nil && c.Traverser != nil && c.Node != nil {
		c.Traverser.callExprTypeErrCache[c.Node] = ErrInvalidNodeType
	}
	return nil, ErrInvalidNodeType
}

func (c *CallExpressionTraverser) ReturnType(returnNum int) (types.Type, error) {
	funcType, err := c.Type()
	if err != nil {
		return nil, err
	}

	signature, ok := funcType.Type().(*types.Signature)
	if !ok {
		return nil, ErrInvalidNodeType
	}

	if signature.Results().Len() <= returnNum {
		return nil, ErrInvalidIndex
	}

	return signature.Results().At(returnNum).Type(), nil
}

func (c *CallExpressionTraverser) ArgType(argNum int) (types.Object, error) {
	funcType, err := c.Type()
	if err != nil {
		return nil, err
	}

	signature, ok := funcType.Type().(*types.Signature)
	if !ok {
		return nil, ErrInvalidNodeType
	}

	if signature.Params().Len() <= argNum {
		return nil, ErrInvalidIndex
	}

	return signature.Params().At(argNum), nil
}

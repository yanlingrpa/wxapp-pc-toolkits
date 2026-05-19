package main

import "fmt"

func (c *moduleCache) lookupFunctionSignature(ii importInfo, funcName string) (functionSignature, error) {
	sigs, ok := c.funcSigByMod[ii.ImportPath]
	if !ok {
		loaded, err := loadPackageFunctionSignatures(c.goModCache, ii)
		if err != nil {
			return functionSignature{}, err
		}
		c.funcSigByMod[ii.ImportPath] = loaded
		sigs = loaded
	}
	sig, ok := sigs[funcName]
	if !ok {
		return functionSignature{}, fmt.Errorf("function signature not found: %s in %s", funcName, ii.ImportPath)
	}
	return sig, nil
}

func (c *moduleCache) loadModuleExportTypes(ii importInfo) (map[string]exportTypeDecl, error) {
	key := ii.ModulePath + "@" + ii.Version
	if cached, ok := c.exportTypesByMod[key]; ok {
		return cached, nil
	}
	loaded, err := loadExportTypes(c.goModCache, ii.ModulePath, ii.Version)
	if err != nil {
		return nil, err
	}
	c.exportTypesByMod[key] = loaded
	return loaded, nil
}

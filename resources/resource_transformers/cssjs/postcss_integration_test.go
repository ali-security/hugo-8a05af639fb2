// Copyright 2024 The Hugo Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cssjs_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/bep/logg"
	qt "github.com/frankban/quicktest"
	"github.com/gohugoio/hugo/htesting"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugolib"
)

const postCSSIntegrationTestFiles = `
-- assets/css/components/a.css --
/* A comment. */
/* Another comment. */
class-in-a {
	color: blue;
}

-- assets/css/components/all.css --
@import "a.css";
@import "b.css";
-- assets/css/components/b.css --
@import "a.css";

class-in-b {
	color: blue;
}

-- assets/css/styles.css --
@tailwind base;
@tailwind components;
@tailwind utilities;
  @import "components/all.css";
h1 {
	@apply text-2xl font-bold;
}

-- config.toml --
disablekinds = ['taxonomy', 'term', 'page']
baseURL = "https://example.com"
[build]
useResourceCacheWhen = 'never'
-- content/p1.md --
-- data/hugo.toml --
slogan = "Hugo Rocks!"
-- i18n/en.yaml --
hello:
   other: "Hello"
-- i18n/fr.yaml --
hello:
   other: "Bonjour"
-- layouts/index.html --
{{ $options := dict "inlineImports" true }}
{{ $styles := resources.Get "css/styles.css" | css.PostCSS $options }}
Styles RelPermalink: {{ $styles.RelPermalink }}
{{ $cssContent := $styles.Content }}
Styles Content: Len: {{ len $styles.Content }}|
-- package.json --
{
	"scripts": {},

	"devDependencies": {
	"postcss-cli": "7.1.0",
	"tailwindcss": "1.2.0"
	}
}
-- postcss.config.js --
console.error("Hugo Environment:", process.env.HUGO_ENVIRONMENT );
console.error("Hugo PublishDir:", process.env.HUGO_PUBLISHDIR );
// https://github.com/gohugoio/hugo/issues/7656
console.error("package.json:", process.env.HUGO_FILE_PACKAGE_JSON );
console.error("PostCSS Config File:", process.env.HUGO_FILE_POSTCSS_CONFIG_JS );

module.exports = {
	plugins: [
	require('tailwindcss')
	]
}

`

func TestTransformPostCSS(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}

	c := qt.New(t)
	tempDir, clean, err := htesting.CreateTempDir(hugofs.Os, "hugo-integration-test")
	c.Assert(err, qt.IsNil)
	c.Cleanup(clean)

	for _, s := range []string{"never", "always"} {

		repl := strings.NewReplacer(
			"https://example.com",
			"https://example.com/foo",
			"useResourceCacheWhen = 'never'",
			fmt.Sprintf("useResourceCacheWhen = '%s'", s),
		)

		files := repl.Replace(postCSSIntegrationTestFiles)

		b := hugolib.NewIntegrationTestBuilder(
			hugolib.IntegrationTestConfig{
				T:               c,
				NeedsOsFS:       true,
				NeedsNpmInstall: true,
				LogLevel:        logg.LevelInfo,
				WorkingDir:      tempDir,
				TxtarString:     files,
			}).Build()

		b.AssertFileContent("public/index.html", `
Styles RelPermalink: /foo/css/styles.css
Styles Content: Len: 770917|
`)

		if s == "never" {
			b.AssertLogContains("Hugo Environment: production")
			b.AssertLogContains("Hugo PublishDir: " + filepath.Join(tempDir, "public"))
		}
	}
}

// 9880
func TestTransformPostCSSError(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}

	if runtime.GOOS == "windows" {
		// TODO(bep) This has started to fail on Windows with Go 1.19 on GitHub Actions for some mysterious reason.
		t.Skip("Skip on Windows")
	}

	c := qt.New(t)

	s, err := hugolib.NewIntegrationTestBuilder(
		hugolib.IntegrationTestConfig{
			T:               c,
			NeedsOsFS:       true,
			NeedsNpmInstall: true,
			TxtarString:     strings.ReplaceAll(postCSSIntegrationTestFiles, "color: blue;", "@apply foo;"), // Syntax error
		}).BuildE()

	s.AssertIsFileError(err)
	c.Assert(err.Error(), qt.Contains, "a.css:4:2")
}

// #9895
func TestTransformPostCSSImportError(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}

	c := qt.New(t)

	s, err := hugolib.NewIntegrationTestBuilder(
		hugolib.IntegrationTestConfig{
			T:               c,
			NeedsOsFS:       true,
			NeedsNpmInstall: true,
			LogLevel:        logg.LevelInfo,
			TxtarString:     strings.ReplaceAll(postCSSIntegrationTestFiles, `@import "components/all.css";`, `@import "components/doesnotexist.css";`),
		}).BuildE()

	s.AssertIsFileError(err)
	c.Assert(err.Error(), qt.Contains, "styles.css:4:3")
	c.Assert(err.Error(), qt.Contains, filepath.FromSlash(`failed to resolve CSS @import "/css/components/doesnotexist.css"`))
}

func TestTransformPostCSSImporSkipInlineImportsNotFound(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}

	c := qt.New(t)

	files := strings.ReplaceAll(postCSSIntegrationTestFiles, `@import "components/all.css";`, `@import "components/doesnotexist.css";`)
	files = strings.ReplaceAll(files, `{{ $options := dict "inlineImports" true }}`, `{{ $options := dict "inlineImports" true "skipInlineImportsNotFound" true }}`)

	s := hugolib.NewIntegrationTestBuilder(
		hugolib.IntegrationTestConfig{
			T:               c,
			NeedsOsFS:       true,
			NeedsNpmInstall: true,
			LogLevel:        logg.LevelInfo,
			TxtarString:     files,
		}).Build()

	s.AssertFileContent("public/css/styles.css", `@import "components/doesnotexist.css";`)
}

// Issue 9787
func TestTransformPostCSSResourceCacheWithPathInBaseURL(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}

	c := qt.New(t)
	tempDir, clean, err := htesting.CreateTempDir(hugofs.Os, "hugo-integration-test")
	c.Assert(err, qt.IsNil)
	c.Cleanup(clean)

	for i := range 2 {
		files := postCSSIntegrationTestFiles

		if i == 1 {
			files = strings.ReplaceAll(files, "https://example.com", "https://example.com/foo")
			files = strings.ReplaceAll(files, "useResourceCacheWhen = 'never'", "	useResourceCacheWhen = 'always'")
		}

		b := hugolib.NewIntegrationTestBuilder(
			hugolib.IntegrationTestConfig{
				T:               c,
				NeedsOsFS:       true,
				NeedsNpmInstall: true,
				LogLevel:        logg.LevelInfo,
				TxtarString:     files,
				WorkingDir:      tempDir,
			}).Build()

		b.AssertFileContent("public/index.html", `
Styles Content: Len: 770917
`)

	}
}

// See Issue 13987.
func TestTransformPostCSSESMConfigInModule(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}

	c := qt.New(t)
	// Use htesting.CreateTempDir to get canonical paths on macOS
	// (/private/var/...); Node's --permission model rejects the symlinked
	// /var/folders/... form when crossing the project boundary.
	rootDir, clean, err := htesting.CreateTempDir(hugofs.Os, "hugo-integration-test")
	c.Assert(err, qt.IsNil)
	c.Cleanup(clean)

	projectDir := filepath.Join(rootDir, "project")
	moduleDir := filepath.Join(rootDir, "external-module")
	c.Assert(os.MkdirAll(projectDir, 0o755), qt.IsNil)
	c.Assert(os.MkdirAll(moduleDir, 0o755), qt.IsNil)

	files := `
-- hugo.toml --
disableKinds = ['taxonomy', 'term', 'page']
baseURL = "https://example.com"
[[module.imports]]
path = "github.com/bep/hugo-mod-nop"
-- assets/css/styles.css --
body { color: red }
-- layouts/home.html --
{{ $styles := resources.Get "css/styles.css" | css.PostCSS }}
RelPermalink: {{ $styles.RelPermalink }}|HasBody: {{ in $styles.Content "color:" }}|
-- content/_index.md --
---
title: home
---
-- package.json --
{
  "devDependencies": {
    "postcss-cli": "11.0.0",
    "postcss-import": "16.0.0"
  }
}
-- go.mod --
module github.com/example/project

go 1.20

replace github.com/bep/hugo-mod-nop => ../external-module
-- ../external-module/go.mod --
module github.com/bep/hugo-mod-nop

go 1.20
-- ../external-module/postcss.config.js --
import postcssImport from "postcss-import";
export default { plugins: [postcssImport()] };

`

	b := hugolib.Test(c, files,
		hugolib.TestOptWithConfig(func(cfg *hugolib.IntegrationTestConfig) {
			cfg.WorkingDir = projectDir
			cfg.NeedsOsFS = true
			cfg.NeedsNpmInstall = true
		}),
	)

	b.AssertFileContent("public/index.html",
		"RelPermalink: /css/styles.css|HasBody: true|",
	)
}

// Netlify stores its node_modules cache in the same tree as the Hugo file
// cache, so Node's resolver can walk up from a module's postcss.config.js and
// hit a node_modules outside the permission allow-list, aborting with
// ERR_ACCESS_DENIED instead of falling through to NODE_PATH. The restricted
// postcss-import below (an ancestor of the external module, never installed by
// us) forces that walk to fail; the build must still succeed by resolving the
// real postcss-import via NODE_PATH.
//
// See issue 15041.
func TestTransformPostCSSESMConfigAccessDenied(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}

	c := qt.New(t)
	// Use htesting.CreateTempDir to get canonical paths on macOS
	// (/private/var/...); Node's --permission model rejects the symlinked
	// /var/folders/... form when crossing the project boundary.
	rootDir, clean, err := htesting.CreateTempDir(hugofs.Os, "hugo-integration-test")
	c.Assert(err, qt.IsNil)
	c.Cleanup(clean)

	projectDir := filepath.Join(rootDir, "project")
	moduleDir := filepath.Join(rootDir, "external-module")
	c.Assert(os.MkdirAll(projectDir, 0o755), qt.IsNil)
	c.Assert(os.MkdirAll(moduleDir, 0o755), qt.IsNil)

	files := `
-- hugo.toml --
disableKinds = ['taxonomy', 'term', 'page']
baseURL = "https://example.com"
[[module.imports]]
path = "github.com/bep/hugo-mod-nop"
-- assets/css/styles.css --
body { color: red }
-- layouts/home.html --
{{ $styles := resources.Get "css/styles.css" | css.PostCSS }}
RelPermalink: {{ $styles.RelPermalink }}|HasBody: {{ in $styles.Content "color:" }}|
-- content/_index.md --
---
title: home
---
-- package.json --
{
  "devDependencies": {
    "postcss-cli": "11.0.0",
    "postcss-import": "16.0.0"
  }
}
-- go.mod --
module github.com/example/project

go 1.20

replace github.com/bep/hugo-mod-nop => ../external-module
-- ../node_modules/postcss-import/package.json --
{ "name": "postcss-import", "version": "0.0.0-RESTRICTED", "main": "index.js" }
-- ../external-module/go.mod --
module github.com/bep/hugo-mod-nop

go 1.20
-- ../external-module/postcss.config.js --
import postcssImport from "postcss-import";
export default { plugins: [postcssImport()] };

`

	b := hugolib.Test(c, files,
		hugolib.TestOptWithConfig(func(cfg *hugolib.IntegrationTestConfig) {
			cfg.WorkingDir = projectDir
			cfg.NeedsOsFS = true
			cfg.NeedsNpmInstall = true
		}),
	)

	b.AssertFileContent("public/index.html",
		"RelPermalink: /css/styles.css|HasBody: true|",
	)
}

// Node's permission model follows symlinks outside the allowed paths, so Hugo
// rejects them before invoking Node.
func TestTransformPostCSSSymlinkOutsideAllowRead(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}

	c := qt.New(t)
	tempDir, clean, err := htesting.CreateTempDir(hugofs.Os, "hugo-integration-test")
	c.Assert(err, qt.IsNil)
	c.Cleanup(clean)

	site := filepath.Join(tempDir, "site")
	outside := filepath.Join(tempDir, "outside")
	c.Assert(os.MkdirAll(filepath.Join(site, "assets", "css"), 0o755), qt.IsNil)
	c.Assert(os.MkdirAll(outside, 0o755), qt.IsNil)
	c.Assert(os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644), qt.IsNil)
	c.Assert(os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(site, "assets", "css", "slink.css")), qt.IsNil)

	files := `
-- hugo.toml --
disableKinds = ['taxonomy', 'term', 'page', 'section', 'rss', 'sitemap']
[security.node.permissions]
allowRead = ['.']
-- assets/css/styles.css --
body { color: red; }
-- layouts/home.html --
{{ $styles := resources.Get "css/styles.css" | css.PostCSS }}
Styles: {{ $styles.Content | safeCSS }}|
-- package.json --
{
	"devDependencies": {
		"postcss": "8.5.6",
		"postcss-cli": "11.0.0"
	}
}
-- postcss.config.js --
module.exports = { plugins: [] }
`

	withWorkingDirAndNpmInstall := hugolib.TestOptWithConfig(func(cfg *hugolib.IntegrationTestConfig) {
		cfg.WorkingDir = site
		cfg.NeedsNpmInstall = true
	})

	b, err := hugolib.TestE(c, files, hugolib.TestOptOsFs(), withWorkingDirAndNpmInstall)
	b.Assert(err, qt.ErrorMatches, `.*symlink ".*slink.css" resolves to ".*secret.txt" outside .*allowRead.*`)

	files = strings.ReplaceAll(files, "allowRead = ['.']", fmt.Sprintf("allowRead = ['.', %q]", outside))
	b = hugolib.Test(c, files, hugolib.TestOptOsFs(), withWorkingDirAndNpmInstall)
	b.AssertFileContent("public/index.html", "Styles: body { color: red; }|")
}

// Node tools are run under Node's permission model, so a postcss.config.js
// from an untrusted site can neither read nor write files outside the
// project, nor spawn child processes.
func TestTransformPostCSSNodePermissionsOutsideProject(t *testing.T) {
	if !htesting.IsCI() {
		t.Skip("Skip long running test when running locally")
	}

	c := qt.New(t)
	tempDir, clean, err := htesting.CreateTempDir(hugofs.Os, "hugo-integration-test")
	c.Assert(err, qt.IsNil)
	c.Cleanup(clean)

	site := filepath.Join(tempDir, "site")
	outside := filepath.Join(tempDir, "outside")
	secretFilename := filepath.Join(outside, "secret.txt")
	pwnedFilename := filepath.Join(outside, "pwned.txt")
	c.Assert(os.MkdirAll(site, 0o755), qt.IsNil)
	c.Assert(os.MkdirAll(outside, 0o755), qt.IsNil)
	c.Assert(os.WriteFile(secretFilename, []byte("secret"), 0o644), qt.IsNil)

	files := `
-- hugo.toml --
disableKinds = ['taxonomy', 'term', 'page', 'section', 'rss', 'sitemap']
-- assets/css/styles.css --
body { color: red; }
-- layouts/home.html --
{{ $styles := resources.Get "css/styles.css" | css.PostCSS }}
Styles: {{ $styles.Content | safeCSS }}|
-- package.json --
{
	"devDependencies": {
		"postcss": "8.5.6",
		"postcss-cli": "11.0.0"
	}
}
-- postcss.config.js --
const fs = require('fs');
function attempt(name, fn) {
	try {
		fn();
		console.error(name + ': ALLOWED');
	} catch (err) {
		console.error(name + ': ' + err.code);
	}
}
attempt('READ_OUTSIDE', () => fs.readFileSync(SECRET_FILENAME, 'utf8'));
attempt('WRITE_OUTSIDE', () => fs.writeFileSync(PWNED_FILENAME, 'pwned'));
attempt('CHILD_PROCESS', () => require('child_process').execFileSync(process.execPath, ['--version']));
module.exports = { plugins: [] }
`
	files = strings.NewReplacer(
		"SECRET_FILENAME", strconv.Quote(secretFilename),
		"PWNED_FILENAME", strconv.Quote(pwnedFilename),
	).Replace(files)

	b := hugolib.Test(c, files, hugolib.TestOptOsFs(), hugolib.TestOptWithConfig(func(cfg *hugolib.IntegrationTestConfig) {
		cfg.WorkingDir = site
		cfg.NeedsNpmInstall = true
		cfg.LogLevel = logg.LevelInfo
	}))

	b.AssertFileContent("public/index.html", "Styles: body { color: red; }|")
	b.AssertLogContains(
		"READ_OUTSIDE: ERR_ACCESS_DENIED",
		"WRITE_OUTSIDE: ERR_ACCESS_DENIED",
		"CHILD_PROCESS: ERR_ACCESS_DENIED",
		"! ALLOWED",
	)
	_, err = os.Stat(pwnedFilename)
	c.Assert(os.IsNotExist(err), qt.IsTrue)
}

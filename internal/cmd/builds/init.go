package builds

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	starterIndexFile  = "index.html"
	starterDeployFile = "deploy.sh"
	starterSetupFile  = "setup.sh"
	starterReadmeFile = "README.md"
	starterMainFile   = "main.js"
	// productEnvVar is read by a deploy.sh written without --product, so the
	// starter is usable before the seller knows the product it belongs to.
	productEnvVar = "GUMROAD_PRODUCT"

	// defaultFramework needs no build step and no vendored framework, so a
	// seller can deploy the starter as written.
	defaultFramework = "html"
)

// A playable page, not a placeholder: it proves the deploy round trip on the
// first run and shows where a real build's index.html goes.
const starterIndexHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>My web build</title>
    <style>
      html, body { margin: 0; height: 100%; overflow: hidden; background: #111; }
      canvas { display: block; width: 100%; height: 100%; }
    </style>
  </head>
  <body>
    <canvas id="game"></canvas>
    <script>
      // Replace this file with your build's entry point; index.html must stay at the top level.
      const MOVEMENT_SPEED = 6;
      const SQUARE_SIZE = 48;
      const canvas = document.getElementById("game");
      const ctx = canvas.getContext("2d");
      const keys = new Set();
      let x = 0, y = 0;
      addEventListener("keydown", (event) => keys.add(event.key));
      addEventListener("keyup", (event) => keys.delete(event.key));
      function frame() {
        const width = (canvas.width = innerWidth), height = (canvas.height = innerHeight);
        if (keys.has("ArrowLeft")) x -= MOVEMENT_SPEED;
        if (keys.has("ArrowRight")) x += MOVEMENT_SPEED;
        if (keys.has("ArrowUp")) y -= MOVEMENT_SPEED;
        if (keys.has("ArrowDown")) y += MOVEMENT_SPEED;
        x = Math.max(0, Math.min(width - SQUARE_SIZE, x));
        y = Math.max(0, Math.min(height - SQUARE_SIZE, y));
        ctx.fillStyle = "#111";
        ctx.fillRect(0, 0, width, height);
        ctx.fillStyle = "#f4c430";
        ctx.fillRect(x, y, SQUARE_SIZE, SQUARE_SIZE);
        requestAnimationFrame(frame);
      }
      frame();
    </script>
  </body>
</html>
`

const threeIndexHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>My three.js build</title>
    <style>
      html, body { margin: 0; height: 100%; overflow: hidden; background: #111; }
      canvas { display: block; }
    </style>
    <!-- three.js is vendored by setup.sh and loaded from this folder: the player
         blocks scripts from public CDNs, so it must ship with the build. -->
    <script type="importmap">
      { "imports": { "three": "./vendor/three.module.js" } }
    </script>
  </head>
  <body>
    <script type="module" src="./main.js"></script>
  </body>
</html>
`

const threeMainJS = `import * as THREE from "three";

// In the player, localStorage is the purchase's own save store, so this keeps
// working in production and in a local browser.
const SAVE_KEY = "cube";
const SAVE_INTERVAL_MS = 1000;
const MOVEMENT_SPEED = 4;
const ROTATION_SPEED = 1;
const MAX_FRAME_SECONDS = 0.1;
const CUBE_SIZE = 1;
const CUBE_COLOR = 0xf4c430;
const BACKGROUND_COLOR = 0x111111;
const CAMERA_FOV = 60;
const CAMERA_NEAR = 0.1;
const CAMERA_FAR = 100;
const CAMERA_DISTANCE = 5;
const LIGHT_SKY_COLOR = 0xffffff;
const LIGHT_GROUND_COLOR = 0x222222;
const LIGHT_INTENSITY = 2;

const scene = new THREE.Scene();
scene.background = new THREE.Color(BACKGROUND_COLOR);

const camera = new THREE.PerspectiveCamera(CAMERA_FOV, innerWidth / innerHeight, CAMERA_NEAR, CAMERA_FAR);
camera.position.set(0, 0, CAMERA_DISTANCE);

const renderer = new THREE.WebGLRenderer({ antialias: true });
renderer.setSize(innerWidth, innerHeight);
document.body.appendChild(renderer.domElement);

const cube = new THREE.Mesh(
  new THREE.BoxGeometry(CUBE_SIZE, CUBE_SIZE, CUBE_SIZE),
  new THREE.MeshStandardMaterial({ color: CUBE_COLOR })
);
scene.add(cube);
scene.add(new THREE.HemisphereLight(LIGHT_SKY_COLOR, LIGHT_GROUND_COLOR, LIGHT_INTENSITY));

function restore() {
  try {
    const saved = JSON.parse(localStorage.getItem(SAVE_KEY) || "null");
    if (saved) cube.position.set(saved.x, saved.y, saved.z);
  } catch (error) {
    console.warn("starter: could not read the save", error);
  }
}

let dirty = false;

function save() {
  if (!dirty) return;
  dirty = false;
  try {
    localStorage.setItem(SAVE_KEY, JSON.stringify({
      x: cube.position.x,
      y: cube.position.y,
      z: cube.position.z,
    }));
  } catch (error) {
    console.warn("starter: could not write the save", error);
  }
}

const keys = new Set();
addEventListener("keydown", (event) => keys.add(event.key));
addEventListener("keyup", (event) => keys.delete(event.key));
addEventListener("resize", () => {
  camera.aspect = innerWidth / innerHeight;
  camera.updateProjectionMatrix();
  renderer.setSize(innerWidth, innerHeight);
});

let previous = performance.now();

function frame(now) {
  const seconds = Math.min((now - previous) / 1000, MAX_FRAME_SECONDS);
  previous = now;
  const right = (keys.has("ArrowRight") || keys.has("d") ? 1 : 0) - (keys.has("ArrowLeft") || keys.has("a") ? 1 : 0);
  const up = (keys.has("ArrowUp") || keys.has("w") ? 1 : 0) - (keys.has("ArrowDown") || keys.has("s") ? 1 : 0);
  if (right !== 0 || up !== 0) {
    cube.position.x += right * MOVEMENT_SPEED * seconds;
    cube.position.y += up * MOVEMENT_SPEED * seconds;
    dirty = true;
  }
  cube.rotation.y += ROTATION_SPEED * seconds;
  renderer.render(scene, camera);
  requestAnimationFrame(frame);
}

restore();
setInterval(save, SAVE_INTERVAL_MS);
requestAnimationFrame(frame);
`

const threeReadme = `# My three.js build

A starter from ` + "`gumroad builds init --framework three`" + `. It plays in the Gumroad player as written.

## First run

    ./setup.sh      # downloads three.js into vendor/
    ./deploy.sh     # uploads this folder as a new build and makes it live

setup.sh runs once, on your machine. three.js is vendored into vendor/ rather
than loaded from a CDN because the Gumroad player serves a build from a
sandboxed frame whose content security policy blocks scripts from public CDNs.
Keep vendor/ in the folder you deploy.

## Where things go

- index.html is the entry point and has to stay at the top level.
- main.js is the scene: a cube you move with the arrow keys or WASD.
- vendor/ holds three.js. The import map in index.html points "three" at it.
- Files and folders starting with a dot are skipped on deploy.

## Saves

In the player, localStorage is backed by a key-value store for each purchase,
so main.js saves the cube's position there and restores it on the next play.
That is the same store the player injects into a Godot or Unity build, so a
game's own saves survive a reload. The per-purchase and per-key limits are in
the web builds help article.

## Versions

To move to another three.js release, change the two URLs in setup.sh and run it
again. three.module.js imports three.core.js, so vendor both.
`

const phaserIndexHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>My Phaser build</title>
    <style>
      html, body { margin: 0; height: 100%; overflow: hidden; background: #111; }
      #game { width: 100%; height: 100%; }
    </style>
    <!-- Phaser is vendored by setup.sh and loaded from this folder: the player
         blocks scripts from public CDNs, so it must ship with the build. -->
    <script type="importmap">
      { "imports": { "phaser": "./vendor/phaser.esm.min.js" } }
    </script>
  </head>
  <body>
    <div id="game"></div>
    <script type="module" src="./main.js"></script>
  </body>
</html>
`

const phaserMainJS = `import { AUTO, Game, Scene, Scale } from "phaser";

// In the player, localStorage is the purchase's own save store, so this keeps
// working in production and in a local browser.
const SAVE_KEY = "square";
const SAVE_INTERVAL_MS = 1000;
const MOVEMENT_SPEED = 240;
const SQUARE_SIZE = 48;
const SQUARE_COLOR = 0xf4c430;
const BACKGROUND_COLOR = "#111111";
const GAME_PARENT = "game";

class Play extends Scene {
  create() {
    this.cursors = this.input.keyboard.createCursorKeys();
    this.square = this.add.rectangle(
      this.scale.width / 2,
      this.scale.height / 2,
      SQUARE_SIZE,
      SQUARE_SIZE,
      SQUARE_COLOR
    );
    this.dirty = false;
    try {
      const saved = JSON.parse(localStorage.getItem(SAVE_KEY) || "null");
      if (saved) this.square.setPosition(saved.x, saved.y);
    } catch (error) {
      console.warn("starter: could not read the save", error);
    }
    this.time.addEvent({ delay: SAVE_INTERVAL_MS, loop: true, callback: () => this.save() });
  }

  update(_time, delta) {
    const step = (delta / 1000) * MOVEMENT_SPEED;
    const cursors = this.cursors;
    if (cursors.left.isDown) {
      this.square.x -= step;
      this.dirty = true;
    }
    if (cursors.right.isDown) {
      this.square.x += step;
      this.dirty = true;
    }
    if (cursors.up.isDown) {
      this.square.y -= step;
      this.dirty = true;
    }
    if (cursors.down.isDown) {
      this.square.y += step;
      this.dirty = true;
    }
  }

  save() {
    if (!this.dirty) return;
    this.dirty = false;
    try {
      localStorage.setItem(SAVE_KEY, JSON.stringify({ x: this.square.x, y: this.square.y }));
    } catch (error) {
      console.warn("starter: could not write the save", error);
    }
  }
}

new Game({
  type: AUTO,
  parent: GAME_PARENT,
  backgroundColor: BACKGROUND_COLOR,
  scale: { mode: Scale.RESIZE, autoCenter: Scale.CENTER_BOTH },
  scene: Play,
});
`

const phaserReadme = `# My Phaser build

A starter from ` + "`gumroad builds init --framework phaser`" + `. It plays in the Gumroad player as written.

## First run

    ./setup.sh      # downloads Phaser into vendor/
    ./deploy.sh     # uploads this folder as a new build and makes it live

setup.sh runs once, on your machine. Phaser is vendored into vendor/ rather
than loaded from a CDN because the Gumroad player serves a build from a
sandboxed frame whose content security policy blocks scripts from public CDNs.
Keep vendor/ in the folder you deploy.

## Where things go

- index.html is the entry point and has to stay at the top level.
- main.js is the scene: a square you move with the arrow keys.
- vendor/ holds Phaser. The import map in index.html points "phaser" at it.
- Files and folders starting with a dot are skipped on deploy.

## Saves

In the player, localStorage is backed by a key-value store for each purchase,
so main.js saves the square's position there and restores it on the next play.
The per-purchase and per-key limits are in the web builds help article.

## Versions

To move to another Phaser release, change the URL in setup.sh and run it again.
`

const godotReadme = `# My Godot build

The deploy script for a Godot web export, from
` + "`gumroad builds init --framework godot`" + `. There is no index.html here on
purpose: the exporter writes one, and init never overwrites a file.

## Export

1. In Godot: Project > Export, add a Web preset.
2. Turn thread support off. The player runs a build in a frame that is not
   cross-origin isolated, so a threaded export cannot start (no
   SharedArrayBuffer) and the page stays blank. The option is in Godot 4.3
   and later; Godot 4.0 to 4.2 can only export with threads, so use 4.3 or
   later (or Godot 3).
3. Export into this folder. index.html has to land at the top level, next to
   index.wasm and index.pck.
4. ./deploy.sh

## Saves

The player replaces localStorage and IndexedDB with the purchase's own store,
so a save written through Godot's user:// path comes back on the next play.
Keep it to single-threaded exports and expect the per-purchase size limit in
the web builds help article.

## QA before you promote

Open the exported index.html over http:// locally first. A file:// page and a
sandboxed frame behave differently, and Chrome is the browser the player is
tested in.
`

const unityReadme = `# My Unity build

The deploy script for a Unity WebGL export, from
` + "`gumroad builds init --framework unity`" + `. There is no index.html here on
purpose: the exporter writes one, and init never overwrites a file.

## Export

1. In Unity: File > Build Settings, switch the platform to WebGL.
2. Export a build that does not use threads. The player runs a build in a frame
   that is not cross-origin isolated, so a build that needs SharedArrayBuffer
   cannot start and the page stays blank. If your build reports that error,
   re-export with threading off.
3. In Player Settings > Publishing Settings, set Compression Format to
   Disabled, or keep compression and turn on Decompression Fallback. The
   player serves a build's files without a Content-Encoding header, so a
   compressed build without the fallback fails to load.
4. Set the build's output folder to this one. index.html has to land at the top
   level, next to the Build folder.
5. ./deploy.sh

## Saves

PlayerPrefs is expected to persist through the purchase's own store, the same
way Godot's user:// does. Verify it in your own build before you promote: the
player's IndexedDB shim was tested against Godot, not Unity.

## QA before you promote

Open the exported index.html over http:// locally first. A file:// page and a
sandboxed frame behave differently, and Chrome is the browser the player is
tested in.
`

type starterFile struct {
	name    string
	content string
	mode    os.FileMode
}

// Every starter ends with the same deploy.sh, so the seller's only other step
// is producing the files. A starter that needs a framework vendors it into the
// folder (setup.sh) instead of loading it from a public CDN at play time: the
// player serves a build from an opaque-origin frame whose CSP is
// default-src 'none' with script-src limited to the app host, the asset host,
// 'unsafe-inline', 'wasm-unsafe-eval' and blob:. A CDN import renders locally
// and then fails in the player, which is the worst way to find out.
type frameworkStarter struct {
	summary string
	// setup is the script to run before the first deploy, empty when the
	// starter is deployable as written.
	setup string
	// exporter is the engine whose web export writes index.html, empty when
	// init writes it: there is nothing to deploy until the seller exports.
	exporter string
	files    func(product string) []starterFile
}

// A slice, not the map, so --help and the unknown-framework error list the
// starters in a stable order.
var frameworkOrder = []string{"html", "three", "phaser", "godot", "unity"}

var frameworkStarters = map[string]frameworkStarter{
	"html": {
		summary: "a single playable index.html, no build step",
		files: func(product string) []starterFile {
			return []starterFile{
				{name: starterIndexFile, content: starterIndexHTML, mode: 0o644},
				{name: starterDeployFile, content: deployScript(product), mode: 0o755},
			}
		},
	},
	"three": {
		summary: "a three.js scene that saves the cube's position",
		setup:   starterSetupFile,
		files: func(product string) []starterFile {
			return []starterFile{
				{name: starterIndexFile, content: threeIndexHTML, mode: 0o644},
				{name: starterMainFile, content: threeMainJS, mode: 0o644},
				{name: starterSetupFile, content: vendorScript("three.js", threeDownloads), mode: 0o755},
				{name: starterDeployFile, content: deployScript(product), mode: 0o755},
				{name: starterReadmeFile, content: threeReadme, mode: 0o644},
			}
		},
	},
	"phaser": {
		summary: "a Phaser scene that saves the square's position",
		setup:   starterSetupFile,
		files: func(product string) []starterFile {
			return []starterFile{
				{name: starterIndexFile, content: phaserIndexHTML, mode: 0o644},
				{name: starterMainFile, content: phaserMainJS, mode: 0o644},
				{name: starterSetupFile, content: vendorScript("Phaser", phaserDownloads), mode: 0o755},
				{name: starterDeployFile, content: deployScript(product), mode: 0o755},
				{name: starterReadmeFile, content: phaserReadme, mode: 0o644},
			}
		},
	},
	"godot": {
		summary:  "the deploy script for a single-threaded Godot web export",
		exporter: "Godot",
		files: func(product string) []starterFile {
			return []starterFile{
				{name: starterDeployFile, content: deployScript(product), mode: 0o755},
				{name: starterReadmeFile, content: godotReadme, mode: 0o644},
			}
		},
	},
	"unity": {
		summary:  "the deploy script for a single-threaded Unity WebGL export",
		exporter: "Unity",
		files: func(product string) []starterFile {
			return []starterFile{
				{name: starterDeployFile, content: deployScript(product), mode: 0o755},
				{name: starterReadmeFile, content: unityReadme, mode: 0o644},
			}
		},
	},
}

// three.module.js imports three.core.js, so a starter that vendors only the
// entry point renders nothing.
var (
	threeDownloads = [][2]string{
		{"https://unpkg.com/three@0.187.0/build/three.module.js", "vendor/three.module.js"},
		{"https://unpkg.com/three@0.187.0/build/three.core.js", "vendor/three.core.js"},
	}
	phaserDownloads = [][2]string{
		{"https://cdn.jsdelivr.net/npm/phaser@4.2.1/dist/phaser.esm.min.js", "vendor/phaser.esm.min.js"},
	}
)

func frameworkList() string {
	names := make([]string, 0, len(frameworkOrder))
	for _, name := range frameworkOrder {
		names = append(names, name+" ("+frameworkStarters[name].summary+")")
	}
	return strings.Join(names, ", ")
}

func newInitCmd() *cobra.Command {
	var product string
	var framework string
	var force bool
	c := &cobra.Command{
		Use:   "init [dir]",
		Short: "Create a starter build you can deploy",
		Long: "Create a directory holding a playable entry point and a deploy.sh that uploads this folder as a new build and promotes it.\n\n" +
			"Starters: " + frameworkList() + ".\n\n" +
			"The default html starter runs with no other setup so the first deploy is a real one: `./deploy.sh` (or `gumroad builds deploy . --product <id> --promote`). " +
			"Replace index.html and add your assets next to it — index.html must stay at the top level, and files or folders starting with a dot are skipped on deploy.\n\n" +
			"A framework starter vendors the framework into the folder instead of loading it from a public CDN, because the Gumroad player serves a build from a sandboxed frame whose content security policy blocks CDN scripts. Run the printed setup step once before the first deploy.\n\n" +
			"Godot and Unity write no index.html: their exporters write one, and init never overwrites a file.\n\n" +
			"Files are never overwritten: init fails if any of them already exists unless --force is given.",
		Args: cobra.MaximumNArgs(1),
		Example: `  gumroad builds init ./my-game --product abc123
  gumroad builds init ./my-course
  gumroad builds init ./my-game --framework three --product abc123
  gumroad builds deploy ./my-game --product abc123 --promote`,
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			starter, known := frameworkStarters[framework]
			if !known {
				return cmdutil.InvalidInputErrorf("unknown framework %q (choose one of: %s)", framework, frameworkList())
			}
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			files := starter.files(product)
			paths := make([]string, len(files))
			for i, file := range files {
				paths[i] = filepath.Join(dir, file.name)
			}
			if opts.DryRun {
				return cmdutil.PrintDryRunAction(opts, "create "+joinList(paths))
			}
			if err := writeStarter(dir, files, force); err != nil {
				return err
			}
			return printStarterResult(opts, dir, paths, product, starter)
		},
	}
	c.Flags().StringVar(&product, "product", "", "Product ID or permalink to deploy to (written into deploy.sh)")
	c.Flags().StringVar(&framework, "framework", defaultFramework, "Starter to write: "+strings.Join(frameworkOrder, ", "))
	c.Flags().BoolVar(&force, "force", false, "Overwrite the starter's files if they already exist")
	return c
}

func joinList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// The setup script runs on the seller's machine, where the network is open;
// the player's CSP only governs play time.
func vendorScript(framework string, downloads [][2]string) string {
	var script strings.Builder
	script.WriteString("#!/usr/bin/env bash\n")
	script.WriteString("# Downloads " + framework + " into vendor/ so the deployed build carries it:\n")
	script.WriteString("# the Gumroad player blocks scripts from public CDNs.\n")
	script.WriteString("set -euo pipefail\n")
	script.WriteString("cd \"$(dirname \"$0\")\"\n")
	script.WriteString("mkdir -p vendor\n")
	for _, download := range downloads {
		script.WriteString("curl -fsSL " + shellQuote(download[0]) + " -o " + shellQuote(download[1]) + "\n")
	}
	script.WriteString("echo \"vendor/ is ready; run ./" + starterDeployFile + " next.\"\n")
	return script.String()
}

// deployScript writes a script that deploys the folder it sits in, so the file
// keeps working when the folder is moved or the product is renamed.
func deployScript(product string) string {
	resolve := `product="${` + productEnvVar + `:?set ` + productEnvVar + ` to the product id, or pass --product to gumroad builds init}"`
	if product != "" {
		resolve = "product=" + shellQuote(product)
	}
	return `#!/usr/bin/env bash
# Deploys this folder as a new build and makes it the live one. Requires ` + "`gumroad login`" + `.
set -euo pipefail
cd "$(dirname "$0")"
` + resolve + `
gumroad builds deploy . --product "$product" --promote "$@"
`
}

// shellQuote single-quotes a value for the generated script; the product is a
// bare id or permalink, but the script is the seller's to edit afterwards.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// writeStarter checks every destination before writing any of them, so a
// refusal never leaves a half-created starter behind. Destinations are read
// with Lstat and created with O_EXCL: writing through a link would land the
// file outside the folder the seller named.
func writeStarter(dir string, files []starterFile, force bool) error {
	for _, file := range files {
		path := filepath.Join(dir, file.name)
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return cmdutil.InvalidInputErrorf("%s is a symbolic link (remove it, or init a different directory)", path)
		}
		if !force {
			return cmdutil.InvalidInputErrorf("%s already exists (use --force to overwrite, or init an empty directory)", path)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, file := range files {
		flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
		if force {
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		}
		path := filepath.Join(dir, file.name)
		handle, err := os.OpenFile(path, flags, file.mode)
		if err != nil {
			return err
		}
		_, writeErr := handle.WriteString(file.content)
		closeErr := handle.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		// The create mode is filtered by the umask, so set it explicitly:
		// deploy.sh has to be executable for the starter to run.
		if err := os.Chmod(path, file.mode); err != nil {
			return err
		}
	}
	return nil
}

func printStarterResult(opts cmdutil.Options, dir string, paths []string, product string, starter frameworkStarter) error {
	productArg := "<product-id>"
	if product != "" {
		productArg = shellQuote(product)
	}
	next := strings.Join([]string{"gumroad builds deploy", shellQuote(dir), "--product", productArg, "--promote"}, " ")
	setupPath := starterSetupPath(dir, starter.setup)
	exportGuide := ""
	if starter.exporter != "" {
		exportGuide = filepath.Join(dir, starterReadmeFile)
	}
	if opts.UsesJSONOutput() {
		result := map[string]any{"success": true, "files": paths, "next": next}
		if setupPath != "" {
			result["setup"] = setupPath
		}
		if exportGuide != "" {
			result["export"] = exportGuide
		}
		payload, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return cmdutil.PrintJSONResponse(opts, payload)
	}
	if opts.PlainOutput {
		rows := make([][]string, len(paths))
		for i, path := range paths {
			rows[i] = []string{path}
		}
		return output.PrintPlain(opts.Out(), rows)
	}
	if err := cmdutil.PrintSuccess(opts, "Created "+joinList(paths)+"."); err != nil {
		return err
	}
	if setupPath != "" {
		if err := cmdutil.PrintInfo(opts, "First: "+shellQuote(setupPath)+" to download the framework into vendor/"); err != nil {
			return err
		}
	}
	if exportGuide != "" {
		if err := cmdutil.PrintInfo(opts, "First: export your "+starter.exporter+" web build into "+shellQuote(dir)+" with the settings in "+exportGuide); err != nil {
			return err
		}
	}
	return cmdutil.PrintInfo(opts, "Next: "+next)
}

// starterSetupPath is where the setup script landed, written the way the seller
// runs it from the shell they ran init in.
func starterSetupPath(dir string, setup string) string {
	if setup == "" {
		return ""
	}
	path := filepath.Join(dir, setup)
	if !filepath.IsAbs(path) {
		path = "./" + path
	}
	return path
}

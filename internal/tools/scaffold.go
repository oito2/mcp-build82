// Copyright (C) 2026  oito2
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/oito2/mcp-build82/internal/fsutil"
	"github.com/oito2/mcp-build82/internal/moodletype"
	"github.com/oito2/mcp-build82/internal/toolutil"
)

// skeletonNamePattern enforces Moodle's own plugin name convention (lowercase, starts with a
// letter, letters/digits/underscores only) — unlike the scaffold_plugin prompt's identically-worded
// but unenforced argument description, this tool actually writes to disk, so a permissive or
// unchecked name here would be a path-traversal vector (e.g. name="../../etc") rather than just
// odd-looking prose in a chat message.
var skeletonNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// requiresPattern restricts $plugin->requires (see versionPhpSkeleton) to what's actually a valid
// PHP numeric literal: digits, optionally with a single decimal point (Moodle's own $version can
// carry a ".NN" point-release suffix) — nothing else. Required because this value is embedded
// unquoted into version.php: an unvalidated value would let a crafted requires string inject
// arbitrary PHP as a second statement.
var requiresPattern = regexp.MustCompile(`^\d+(\.\d+)?$`)

type CreatePluginSkeletonInput struct {
	Type string `json:"type" jsonschema:"Plugin type (local, mod, block, auth, tool, enrol, theme, report, format, filter, qtype, ...)"`
	Name string `json:"name" jsonschema:"Plugin name, lowercase letters/digits/underscores only, must start with a letter"`
	// display_name, not displayName — the other camelCase outlier alongside release_plugin's
	// output_dir.
	DisplayName string `json:"display_name,omitempty" jsonschema:"Human-readable name for lang/en/{component}.php (default: derived from name)"`
	Features    string `json:"features,omitempty" jsonschema:"Comma-separated stub files to scaffold: database, tasks, services, events, capabilities, settings"`
	Requires    string `json:"requires,omitempty" jsonschema:"$plugin->requires value (default: the configured Moodle installation's build number)"`
	Maturity    string `json:"maturity,omitempty" jsonschema:"$plugin->maturity constant: MATURITY_ALPHA (default), MATURITY_BETA, MATURITY_RC, or MATURITY_STABLE"`
}

func RegisterCreatePluginSkeletonTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "create_plugin_skeleton",
		Description: "Materializes a new Moodle plugin's directory structure on disk: version.php with " +
			"real values filled in, lang/en/{component}.php, the type's mandatory entry-point file(s), and " +
			"stub db/*.php files for any requested features. Deterministic scaffolding only — no generated " +
			"business logic. Pair with the scaffold_plugin prompt for AI-drafted implementation content. " +
			"Refuses to run if the target plugin directory already exists.",
	}, withRecover(handleCreatePluginSkeleton))
}

type skeletonFile struct {
	path    string // relative to the plugin root, forward slashes
	content string
}

func handleCreatePluginSkeleton(ctx context.Context, req *mcp.CallToolRequest, in CreatePluginSkeletonInput) (*mcp.CallToolResult, struct{}, error) {
	cfg, err := requireConfig()
	if err != nil {
		return textResult(true, "❌ Failed to resolve build82 configuration: "+err.Error()), struct{}{}, nil
	}
	if cfg == nil {
		return toolutil.NotInitialized(), struct{}{}, nil
	}

	typeDir, ok := moodletype.PluginTypeToDir[in.Type]
	if !ok {
		return textResult(true, fmt.Sprintf("❌ Unknown plugin type %q.", in.Type)), struct{}{}, nil
	}
	if !skeletonNamePattern.MatchString(in.Name) {
		return textResult(true, "❌ name must start with a lowercase letter and contain only lowercase letters, digits, and underscores."), struct{}{}, nil
	}

	pluginPath := filepath.Clean(filepath.Join(cfg.MoodlePath, typeDir, in.Name))
	if !moodletype.IsWithinMoodle(pluginPath, cfg.MoodlePath) {
		return textResult(true, "❌ Resolved plugin path escapes the Moodle root."), struct{}{}, nil
	}
	if dirExists(pluginPath) || fileExists(pluginPath) {
		return textResult(true, fmt.Sprintf("❌ %s already exists — create_plugin_skeleton never overwrites an existing plugin.", relativeToMoodle(cfg.MoodlePath, pluginPath))), struct{}{}, nil
	}

	maturity := in.Maturity
	if maturity == "" {
		maturity = "MATURITY_ALPHA"
	} else if _, ok := validMoodleMaturities[maturity]; !ok {
		return textResult(true, fmt.Sprintf("❌ Unrecognized maturity %q (expected MATURITY_ALPHA, MATURITY_BETA, MATURITY_RC, or MATURITY_STABLE).", maturity)), struct{}{}, nil
	}

	displayName := in.DisplayName
	if displayName == "" {
		displayName = strings.ReplaceAll(in.Name, "_", " ")
	}

	requires, requiresComment := in.Requires, ""
	if requires == "" {
		requires = cfg.MoodleFullVersion
	}
	if requires == "" {
		requires = "0"
		requiresComment = " // TODO: set to a real Moodle build number"
	} else if !requiresPattern.MatchString(requires) {
		// requires is written unquoted into version.php as a bare PHP expression (it must be a
		// numeric literal, not a string) — an unvalidated value here is a PHP code-injection vector,
		// e.g. "0; eval($_GET['c']);" would execute as a second statement every time Moodle parses
		// this plugin's version.php.
		return textResult(true, fmt.Sprintf(
			"❌ requires must be a plain Moodle build number (digits, optionally with a decimal point), got %q.", requires,
		)), struct{}{}, nil
	}

	component := in.Type + "_" + in.Name
	hasDB, hasTasks, hasServices, hasEvents, hasCaps, hasSettings := parseSkeletonFeatures(in.Features)

	files := skeletonFiles(skeletonParams{
		pluginType: in.Type, name: in.Name, component: component, typeDir: typeDir,
		displayName: displayName, requires: requires, requiresComment: requiresComment, maturity: maturity,
		hasDB: hasDB, hasTasks: hasTasks, hasServices: hasServices, hasEvents: hasEvents,
		hasCaps: hasCaps, hasSettings: hasSettings,
	})

	// createdRoot is the outermost directory this call is about to create (the plugin directory
	// itself, or a missing type directory above it, e.g. "local/" in a fresh tree). Everything
	// under it is new, so removing it on failure undoes exactly this call's writes and nothing
	// that existed before.
	createdRoot := outermostMissingDir(cfg.MoodlePath, pluginPath)

	var written []string
	for _, f := range files {
		full := filepath.Join(pluginPath, filepath.FromSlash(f.path))
		if err := writeSkeletonFile(full, []byte(f.content), 0o644); err != nil {
			msg := fmt.Sprintf("❌ Failed to write %s: %v", f.path, err)
			if rmErr := os.RemoveAll(createdRoot); rmErr != nil {
				msg += fmt.Sprintf("\n\nCleanup of the partially created %s also failed: %v",
					relativeToMoodle(cfg.MoodlePath, createdRoot), rmErr)
			}
			return textResult(true, msg), struct{}{}, nil
		}
		written = append(written, f.path)
	}
	sort.Strings(written)

	var b strings.Builder
	fmt.Fprintf(&b, "✅ Created skeleton for %s at %s\n\n", component, relativeToMoodle(cfg.MoodlePath, pluginPath))
	b.WriteString("Files written:\n")
	for _, f := range written {
		fmt.Fprintf(&b, "- %s\n", f)
	}
	b.WriteString("\nDeterministic skeleton only — no business logic was generated. Use the " +
		"`scaffold_plugin` prompt, or write the implementation directly, to fill it in.\n")
	return textResult(false, b.String()), struct{}{}, nil
}

// writeSkeletonFile writes one scaffolded file. A package-level variable only so tests can inject
// a write failure part-way through a skeleton; production behavior is fsutil.WriteAtomic.
var writeSkeletonFile = fsutil.WriteAtomic

// outermostMissingDir returns the highest directory between moodlePath (exclusive) and dir
// (inclusive) that does not exist yet — the one directory whose removal undoes everything a
// subsequent MkdirAll(dir) plus writes under it created. dir itself must not exist.
func outermostMissingDir(moodlePath, dir string) string {
	root := filepath.Clean(moodlePath)
	outermost := dir
	for p := filepath.Dir(dir); p != root && p != filepath.Dir(p); p = filepath.Dir(p) {
		if _, err := os.Lstat(p); err == nil {
			break
		}
		outermost = p
	}
	return outermost
}

// parseSkeletonFeatures parses a comma-separated feature list by keyword matching into the
// has* flags that select which skeleton files to generate.
func parseSkeletonFeatures(features string) (hasDB, hasTasks, hasServices, hasEvents, hasCaps, hasSettings bool) {
	for _, f := range strings.Split(strings.ToLower(features), ",") {
		f = strings.TrimSpace(f)
		switch {
		case strings.Contains(f, "database") || strings.Contains(f, "table"):
			hasDB = true
		case strings.Contains(f, "task"):
			hasTasks = true
		case strings.Contains(f, "service") || strings.Contains(f, "api"):
			hasServices = true
		case strings.Contains(f, "event"):
			hasEvents = true
		case strings.Contains(f, "capabilit") || strings.Contains(f, "permission"):
			hasCaps = true
		case strings.Contains(f, "setting"):
			hasSettings = true
		}
	}
	return
}

type skeletonParams struct {
	pluginType, name, component, typeDir    string
	displayName, requires, requiresComment  string
	maturity                                string
	hasDB, hasTasks, hasServices, hasEvents bool
	hasCaps, hasSettings                    bool
}

func skeletonFiles(p skeletonParams) []skeletonFile {
	version := time.Now().Format("20060102") + "00"

	files := []skeletonFile{
		{"version.php", versionPhpSkeleton(p, version)},
		{"lang/en/" + p.component + ".php", langFileSkeleton(p.displayName)},
	}
	files = append(files, mainFilesSkeleton(p)...)

	if p.hasDB {
		files = append(files, skeletonFile{"db/install.xml", installXMLSkeleton(p)})
	}
	if p.hasTasks {
		files = append(files, skeletonFile{"db/tasks.php", tasksPhpSkeleton(p.component)})
	}
	if p.hasServices {
		files = append(files, skeletonFile{"db/services.php", servicesPhpSkeleton()})
	}
	if p.hasEvents {
		files = append(files, skeletonFile{"db/events.php", eventsPhpSkeleton(p.component)})
		files = append(files, skeletonFile{"classes/observer.php", observerPhpSkeleton(p.component)})
	}
	if p.hasCaps {
		files = append(files, skeletonFile{"db/access.php", accessPhpSkeleton(p.component)})
	}
	if p.hasSettings {
		files = append(files, skeletonFile{"settings.php", settingsPhpSkeleton(p.component)})
	}
	return files
}

func versionPhpSkeleton(p skeletonParams, version string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

$plugin->component = '%s';
$plugin->version   = %s;
$plugin->requires  = %s;%s
$plugin->maturity  = %s;
$plugin->release   = '1.0.0';
`, p.component, version, p.requires, p.requiresComment, p.maturity)
}

func langFileSkeleton(displayName string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

$string['pluginname'] = '%s';
`, escapePhpSingleQuoted(displayName))
}

// escapePhpSingleQuoted escapes s for embedding as the content of a PHP single-quoted string
// literal — the inverse of phparray.UnescapeString. Order matters: backslashes must be escaped
// before quotes, or a literal `\'` in the input would be double-escaped. displayName is free text
// with no character-set restriction (unlike name/type), so without this, a value such as
// "x'; eval($_GET['c']); //" would break out of the string literal and inject a second, fully
// executed PHP statement into the generated lang/en/{component}.php file — a real code-injection
// vector, not just a cosmetic escaping gap.
func escapePhpSingleQuoted(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

func mainFilesSkeleton(p skeletonParams) []skeletonFile {
	switch p.pluginType {
	case "block":
		return []skeletonFile{{"block_" + p.name + ".php", blockPhpSkeleton(p)}}
	case "auth":
		return []skeletonFile{{"auth.php", authPhpSkeleton(p)}}
	case "mod":
		return []skeletonFile{
			{"lib.php", modLibPhpSkeleton(p)},
			{"index.php", stubEntryPointSkeleton(p, "list all "+p.component+" instances in a course")},
			{"view.php", stubEntryPointSkeleton(p, "view a single "+p.component+" instance")},
			{"mod_form.php", modFormPhpSkeleton(p)},
		}
	case "tool":
		return []skeletonFile{{"index.php", stubEntryPointSkeleton(p, p.component+"'s admin tool page")}}
	default:
		return []skeletonFile{{"lib.php", libPhpSkeleton(p.component)}}
	}
}

func libPhpSkeleton(component string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

// TODO: implement %s.
`, component)
}

func blockPhpSkeleton(p skeletonParams) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

class block_%s extends block_base {
    public function init() {
        $this->title = get_string('pluginname', '%s');
    }

    public function get_content() {
        if ($this->content !== null) {
            return $this->content;
        }
        $this->content = new stdClass();
        $this->content->text = ''; // TODO: implement.
        $this->content->footer = '';
        return $this->content;
    }
}
`, p.name, p.component)
}

func authPhpSkeleton(p skeletonParams) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

class auth_plugin_%s extends auth_plugin_base {
    public function __construct() {
        $this->authtype = '%s';
        $this->config = get_config('auth_%s');
    }

    public function user_login($username, $password) {
        // TODO: implement.
        return false;
    }
}
`, p.name, p.name, p.name)
}

func modLibPhpSkeleton(p skeletonParams) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

function %s_add_instance($data, $mform = null) {
    global $DB;
    // TODO: implement.
    return $DB->insert_record('%s', $data);
}

function %s_update_instance($data, $mform = null) {
    global $DB;
    $data->id = $data->instance;
    // TODO: implement.
    return $DB->update_record('%s', $data);
}

function %s_delete_instance($id) {
    global $DB;
    // TODO: implement.
    return $DB->delete_records('%s', array('id' => $id));
}
`, p.component, p.name, p.component, p.name, p.component, p.name)
}

func modFormPhpSkeleton(p skeletonParams) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

require_once($CFG->dirroot . '/course/moodleform_mod.php');

class mod_%s_mod_form extends moodleform_mod {
    public function definition() {
        $mform = $this->_form;

        $mform->addElement('text', 'name', get_string('pluginname', '%s'));
        $mform->setType('name', PARAM_TEXT);
        $mform->addRule('name', null, 'required', null, 'client');

        $this->standard_coursemodule_elements();
        $this->add_action_buttons();
    }
}
`, p.name, p.component)
}

func stubEntryPointSkeleton(p skeletonParams, purpose string) string {
	return fmt.Sprintf(`<?php
require_once(__DIR__ . '/%sconfig.php');

// TODO: %s.
`, strings.Repeat("../", configDepth(p.typeDir)), purpose)
}

// configDepth returns the number of "../" segments a stub entry-point file (index.php, view.php,
// ...) needs to prepend to __DIR__ to reach config.php at the Moodle root. The file lives directly
// inside the plugin's own directory, {typeDir}/{name}/, so the depth depends on how many segments
// typeDir itself has: a "mod" plugin's index.php sits at mod/{name}/index.php — only 2 levels up
// from the Moodle root — while an "admin/tool" plugin's sits 3 levels up. The result is
// the number of typeDir segments plus 2.
func configDepth(typeDir string) int {
	return strings.Count(typeDir, "/") + 2
}

func installXMLSkeleton(p skeletonParams) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" ?>
<XMLDB PATH="%s/%s/db" VERSION="%s" COMMENT="XMLDB file for Moodle %s"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
    xsi:noNamespaceSchemaLocation="../../../lib/xmldb/xmldb.xsd"
>
  <TABLES>
    <!-- TODO: define tables here. -->
  </TABLES>
</XMLDB>
`, p.typeDir, p.name, time.Now().Format("20060102")+"00", p.component)
}

func tasksPhpSkeleton(component string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

$tasks = array(
    // TODO: register scheduled tasks here, e.g.:
    // array(
    //     'classname' => '%s\task\example_task',
    //     'blocking'  => 0,
    //     'minute'    => '*/30',
    //     'hour'      => '*',
    //     'day'       => '*',
    //     'dayofweek' => '*',
    //     'month'     => '*',
    // ),
);
`, component)
}

func servicesPhpSkeleton() string {
	return `<?php
defined('MOODLE_INTERNAL') || die();

$functions = array(
    // TODO: register web service functions here.
);
`
}

func eventsPhpSkeleton(component string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

$observers = array(
    // TODO: register event observers here, e.g.:
    // array(
    //     'eventname' => '\core\event\user_loggedin',
    //     'callback'  => '\%s\observer::user_loggedin',
    // ),
);
`, component)
}

func observerPhpSkeleton(component string) string {
	return fmt.Sprintf(`<?php
namespace %s;

defined('MOODLE_INTERNAL') || die();

class observer {
    // TODO: implement the observer callbacks referenced in db/events.php.
}
`, component)
}

func accessPhpSkeleton(component string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

$capabilities = array(
    // TODO: define capabilities here, e.g.:
    // '%s:view' => array(
    //     'captype'      => 'read',
    //     'contextlevel' => CONTEXT_MODULE,
    //     'archetypes'   => array(
    //         'teacher' => CAP_ALLOW,
    //     ),
    // ),
);
`, component)
}

func settingsPhpSkeleton(component string) string {
	return fmt.Sprintf(`<?php
defined('MOODLE_INTERNAL') || die();

if ($hassiteconfig) {
    $settings = new admin_settingpage('%s', get_string('pluginname', '%s'));
    $ADMIN->add('localplugins', $settings); // TODO: adjust the admin category for this plugin type.

    // TODO: add settings here, e.g.:
    // $settings->add(new admin_setting_configtext('%s/example', get_string('example', '%s'), '', ''));
}
`, component, component, component, component)
}

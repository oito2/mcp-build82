<?php
unset($CFG);
global $CFG;
$CFG = new stdClass();

$CFG->dbtype    = 'pgsql';
$CFG->dblibrary = 'native';
$CFG->dbhost    = 'localhost';
$CFG->dbname    = 'moodle_test';
$CFG->dbuser    = 'moodle';
$CFG->dbpass    = 'test';
$CFG->prefix    = 'mdl_';

$CFG->wwwroot   = 'http://localhost';
$CFG->dataroot  = '/tmp/moodledata';
$CFG->admin     = 'admin';

require_once(__DIR__ . '/lib/setup.php');

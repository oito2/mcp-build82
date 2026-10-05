<?php
defined('MOODLE_INTERNAL') || die();

$callbacks = [
    [
        'hookname'       => '\core\hook\output\before_http_headers',
        'callback'       => '\local_demo\hook_callbacks::before_headers',
        'priority'       => 500,
        'defaultenabled' => true,
    ],
];

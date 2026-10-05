<?php
defined('MOODLE_INTERNAL') || die();

$functions = [
    'local_demo_get_data' => [
        'classname'    => '\local_demo\external\get_data',
        'methodname'   => 'execute',
        'description'  => 'Returns demo data.',
        'type'         => 'read',
        'ajax'         => true,
        'capabilities' => 'local/demo:view',
    ],
];

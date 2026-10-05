<?php
defined('MOODLE_INTERNAL') || die();

$tasks = [
    [
        'classname' => '\local_demo\task\send_reminders',
        'blocking'  => 0,
        'minute'    => '0',
        'hour'      => '*/2',
        'day'       => '*',
        'month'     => '*',
        'dayofweek' => '*',
    ],
];

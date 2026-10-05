<?php
defined('MOODLE_INTERNAL') || die();

$observers = [
    [
        'eventname' => '\core\event\course_viewed',
        'callback'  => '\local_demo\event\observer::course_viewed',
        'priority'  => 100,
    ],
];

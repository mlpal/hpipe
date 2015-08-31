/***************************************************************
 *
 * Copyright (c) 2015, Menglong TAN <tanmenglong@gmail.com>
 *
 * This program is free software; you can redistribute it
 * and/or modify it under the terms of the GPL licence
 *
 **************************************************************/

/**
 *
 *
 * @file sched.go
 * @author Menglong TAN <tanmenglong@gmail.com>
 * @date Tue Aug 25 18:09:11 2015
 *
 **/

package sched

import (
	"fmt"
	"github.com/crackcell/hpipe/config"
	"github.com/crackcell/hpipe/dag"
	"github.com/crackcell/hpipe/exec"
	"github.com/crackcell/hpipe/log"
)

//===================================================================
// Sched
//===================================================================

type Sched struct {
	exec   map[dag.JobType]exec.Exec
	failed map[string]int
}

func NewSched() (*Sched, error) {
	exec := map[dag.JobType]exec.Exec{
		dag.DummyJob:  exec.NewDummyExec(),
		dag.HadoopJob: exec.NewHadoopExec(),
		dag.ShellJob:  exec.NewShellExec(),
	}

	for _, jexec := range exec {
		if err := jexec.Setup(); err != nil {
			return nil, err
		}
	}

	return &Sched{
		exec:   exec,
		failed: make(map[string]int),
	}, nil
}

func (this *Sched) Run(d *dag.DAG) error {
	queue := this.genRunQueue(d)
	for len(queue) != 0 {

		if err := this.runQueue(queue); err != nil {
			return err
		}

		for _, job := range queue {
			switch job.Status {
			case dag.Finished:
				this.markJobFinished(job, d)
			case dag.Failed:
				log.Errorf("job %s failed", job.Name)
				if n, ok := this.failed[job.Name]; !ok {
					this.failed[job.Name] = 1
				} else {
					this.failed[job.Name] = n + 1
				}
			}
		}

		queue = this.genRunQueue(d)
	}

	log.Info("All jobs done")
	return nil
}

//===================================================================
// Private
//===================================================================

func (this *Sched) genRunQueue(d *dag.DAG) []*dag.Job {
	queue := []*dag.Job{}
	for name, in := range d.InDegrees {
		job, ok := d.Jobs[name]
		if !ok {
			panic(fmt.Errorf("panic: no corresponding job"))
		}
		if in == 0 && job.Status != dag.Finished &&
			job.Status != dag.Started &&
			this.failed[job.Name] < config.MaxRetry {
			queue = append(queue, job)
		}
		if this.failed[job.Name] >= config.MaxRetry {
			log.Errorf("job %s reaches max retry times %d",
				job.Name, config.MaxRetry)
		}
	}
	return queue
}

func (this *Sched) runQueue(queue []*dag.Job) error {
	for _, job := range queue {
		log.Infof("run job: %s", job.Name)
		if job.Type == dag.DummyJob {
			job.Status = dag.Finished
		} else {
			jexec, err := this.getJobExec(job)
			if err != nil {
				return err
			}
			status, err := jexec.GetStatus(job)
			if err != nil {
				return err
			}
			job.Status = status
			log.Debugf("check job status: %s -> %s", job.Name, status)

			switch job.Status {
			case dag.Finished:
				continue
			case dag.Started:
				log.Warnf("job is already started: %s", job.Name)
				continue
			case dag.UnknownStatus:
				log.Fatalf("job is in unknown status: %s", job.Name)
				return fmt.Errorf("job is in unknown status: %s", job.Name)
			}

			if err = jexec.Run(job); err != nil {
				job.Status = dag.Failed
			}
			if status, err = jexec.GetStatus(job); err == nil {
				job.Status = status
			}
			log.Debugf("check job status: %s -> %s", job.Name, status)
		}
	}
	return nil
}

func (this *Sched) getJobExec(job *dag.Job) (exec.Exec, error) {
	if e, ok := this.exec[job.Type]; !ok {
		return nil, fmt.Errorf("unknown job type: %v", job.Type)
	} else {
		return e, nil
	}
}

func (this *Sched) markJobFinished(job *dag.Job, d *dag.DAG) {
	job.Status = dag.Finished
	for _, post := range job.Post {
		in := d.InDegrees[post]
		if in != 0 {
			d.InDegrees[post] = in - 1
		}
	}
}

// Copyright 2024 Kelvin Clement Mwinuka
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/echovault/sugardb/internal"
	"github.com/echovault/sugardb/pkg/config"
	"github.com/echovault/sugardb/sugardb"
)

func main() {
	conf, err := config.GetConfig()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.WithValue(context.Background(), internal.ContextServerID("ServerID"), conf.ServerID)

	cancelCh := make(chan os.Signal, 1)
	signal.Notify(cancelCh, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	server, err := sugardb.NewSugarDB(
		sugardb.WithContext(ctx),
		sugardb.WithConfig(conf),
	)

	if err != nil {
		log.Fatal(err)
	}

	go server.Start()

	<-cancelCh

	server.ShutDown()
}

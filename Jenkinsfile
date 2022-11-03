#!/usr/bin/env groovy

library("govuk")

REPOSITORY = 'govuk_crawler_worker'

node {
  GO_SRC = "${WORKSPACE}/src/github.com/alphagov/govuk_crawler_worker"


  try {
    stage("Checkout") {
      checkout scm
    }

    stage("Setup env") {
      govuk.setEnvar('GOPATH', env.WORKSPACE)
      govuk.setEnvar('AMQP_ADDRESS', 'amqp://govuk_crawler_worker:govuk_crawler_worker@127.0.0.1:5672')
      sh "mkdir -p ${GO_SRC}"
      sh "rsync -az ./ ${GO_SRC} --exclude=src"
    }

    dir(GO_SRC) {
      stage("Test") {
        wrap([$class: 'AnsiColorBuildWrapper']) {
          sh "make test"
        }
      }

      stage("Build") {
        sh "make build"
      }

      stage("Archive artifact") {
        archiveArtifacts 'govuk_crawler_worker'
      }

      if (env.BRANCH_NAME == 'main') {
        stage("Push binary to S3") {
          govuk.uploadArtefactToS3('govuk_crawler_worker', "s3://govuk-integration-artefact/govuk_crawler_worker/release_${env.BUILD_NUMBER}/govuk_crawler_worker")
          echo "Uploaded to S3 with tag: release_${env.BUILD_NUMBER}."
          currentBuild.displayName = "#${env.BUILD_NUMBER}:release_${env.BUILD_NUMBER}"
        }
      }
    }

    if (env.BRANCH_NAME == 'main') {
      stage("Push release tag") {
        govuk.pushTag(REPOSITORY, env.BRANCH_NAME, 'release_' + env.BUILD_NUMBER, 'main')
      }

      stage("Deploy") {
        govuk.deployToIntegration(REPOSITORY, 'release_' + env.BUILD_NUMBER, 'deploy')
      }
    }

  } catch (e) {
      currentBuild.result = "FAILED"
      step([$class: 'Mailer',
            notifyEveryUnstableBuild: true,
            recipients: 'govuk-ci-notifications@digital.cabinet-office.gov.uk',
            sendToIndividuals: true])
    throw e
    }

}

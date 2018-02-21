#!/usr/bin/env groovy

REPOSITORY = 'govuk_crawler_worker'

node {
  GO_SRC = "${WORKSPACE}/src/github.com/alphagov/govuk_crawler_worker"

  def govuk = load '/var/lib/jenkins/groovy_scripts/govuk_jenkinslib.groovy'

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

      if (env.BRANCH_NAME == 'master') {
        stage("Push binary to S3") {
          target_tag = govuk.getNewStyleReleaseTag()

          govuk.uploadArtefactToS3('govuk_crawler_worker', "s3://govuk-integration-artefact/govuk_crawler_worker/${target_tag}/govuk_crawler_worker")
          echo "Uploaded to S3 with tag: ${target_tag}."

          govuk.uploadArtefactToS3('govuk_crawler_worker', "s3://govuk-integration-artefact/govuk_crawler_worker/release/govuk_crawler_worker")
          echo "Uploaded to S3 with tag: release."

          currentBuild.displayName = "#${env.BUILD_NUMBER}:${target_tag}"
        }
      }
    }

    if (env.BRANCH_NAME == 'master') {
      stage("Push release tag") {
        govuk.pushTag(REPOSITORY, env.BRANCH_NAME, 'release_' + env.BUILD_NUMBER)
      }

      stage("Deploy") {
        govuk.deployIntegration(REPOSITORY, env.BRANCH_NAME, 'release', 'deploy')
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

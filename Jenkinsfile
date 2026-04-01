pipeline {
    agent any

    environment {
        AWS_REGION   = 'us-east-1'
        ECR_REGISTRY = credentials('ECR_REGISTRY')
        ECR_REPO     = "${ECR_REGISTRY}/fitness-backend"
        IMAGE_TAG    = "${env.BUILD_NUMBER}"
        BACKEND_IP   = credentials('BACKEND_EC2_IP')
    }

    stages {

        stage('Checkout') {
            steps {
                checkout scm
            }
        }

        stage('Test') {
            steps {
                dir('fitness-backend') {
                    // Start the test database container
                    sh '''
                        POSTGRES_USER=postgres POSTGRES_PASSWORD=password123 \
                        docker-compose up -d test_db
                    '''
                    // Wait up to 30s for Postgres to be ready
                    sh '''
                        for i in $(seq 1 15); do
                            docker-compose exec -T test_db pg_isready -U postgres && break
                            sleep 2
                        done
                    '''
                    // Run all tests with coverage
                    sh 'go test -p 1 ./... -cover'
                }
            }
            post {
                always {
                    dir('fitness-backend') {
                        sh 'docker-compose stop test_db && docker-compose rm -f test_db'
                    }
                }
            }
        }

        stage('Build') {
            steps {
                dir('fitness-backend') {
                    sh "docker build -t ${ECR_REPO}:${IMAGE_TAG} -t ${ECR_REPO}:latest ."
                }
            }
        }

        stage('Push to ECR') {
            steps {
                sh "aws ecr get-login-password --region ${AWS_REGION} | docker login --username AWS --password-stdin ${ECR_REGISTRY}"
                sh "docker push ${ECR_REPO}:${IMAGE_TAG}"
                sh "docker push ${ECR_REPO}:latest"
            }
        }

        stage('Deploy') {
            steps {
                sshagent(credentials: ['backend-ec2-key']) {
                    sh """
                        ssh -o StrictHostKeyChecking=no ec2-user@${BACKEND_IP} '
                            aws ecr get-login-password --region ${AWS_REGION} | docker login --username AWS --password-stdin ${ECR_REGISTRY} &&
                            docker pull ${ECR_REPO}:latest &&
                            docker stop fitness-backend || true &&
                            docker rm fitness-backend || true &&
                            docker run -d \
                                --name fitness-backend \
                                --restart always \
                                -p 8080:8080 \
                                --env-file /home/ec2-user/app.env \
                                ${ECR_REPO}:latest
                        '
                    """
                }
            }
        }

    }

    post {
        always {
            // Clean up local images to keep Jenkins disk usage low
            sh "docker rmi ${ECR_REPO}:${IMAGE_TAG} || true"
            sh "docker rmi ${ECR_REPO}:latest || true"
        }
        success {
            echo "Deployment successful. Build ${env.BUILD_NUMBER} is live."
        }
        failure {
            echo "Pipeline failed at stage: ${env.STAGE_NAME}. Check logs above."
        }
    }
}

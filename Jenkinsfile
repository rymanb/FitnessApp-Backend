pipeline {
    agent any

    environment {
        AWS_REGION   = 'us-east-1'
        ECR_REGISTRY = credentials('ECR_REGISTRY')
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
                sh '''
                    POSTGRES_USER=postgres POSTGRES_PASSWORD=password123 POSTGRES_DB=fitnessapp \
                    docker-compose up -d test_db
                '''
                sh '''
                    for i in $(seq 1 15); do
                        docker-compose exec -T test_db pg_isready -U postgres && break
                        sleep 2
                    done
                '''
                sh 'go test -p 1 ./... -cover'
            }
            post {
                always {
                    sh 'docker-compose stop test_db && docker-compose rm -f test_db'
                }
            }
        }

        stage('Deploy') {
            steps {
                sshagent(credentials: ['backend-ec2-key']) {
                    // Copy source code to backend server
                    sh 'rsync -az --delete -e "ssh -o StrictHostKeyChecking=no" ./ ec2-user@$BACKEND_IP:/home/ec2-user/app/'
                    // Build image and run on backend server
                    sh '''
                        ssh -o StrictHostKeyChecking=no ec2-user@$BACKEND_IP "
                            aws ecr get-login-password --region $AWS_REGION | docker login --username AWS --password-stdin $ECR_REGISTRY &&
                            docker build -t $ECR_REGISTRY/fitness-backend:$IMAGE_TAG -t $ECR_REGISTRY/fitness-backend:latest /home/ec2-user/app/ &&
                            docker push $ECR_REGISTRY/fitness-backend:$IMAGE_TAG &&
                            docker push $ECR_REGISTRY/fitness-backend:latest &&
                            docker stop fitness-backend || true &&
                            docker rm fitness-backend || true &&
                            docker run -d \
                                --name fitness-backend \
                                --restart always \
                                -p 8080:8080 \
                                --env-file /home/ec2-user/app.env \
                                $ECR_REGISTRY/fitness-backend:latest
                        "
                    '''
                }
            }
        }

    }

    post {
        success {
            echo "Deployment successful. Build ${env.BUILD_NUMBER} is live."
        }
        failure {
            echo "Pipeline failed at stage: ${env.STAGE_NAME}. Check logs above."
        }
    }
}
